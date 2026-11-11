package study

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	radonEmployeesURL   = "https://radon.nauka.gov.pl/opendata/polon/employees"
	radonRequestTimeout = 20 * time.Second
	radonPageSize       = 100
	radonMaxPages       = 200
	radonMaxPageBytes   = 8 << 20
)

type radonClient struct {
	http     *http.Client
	endpoint string
}

func newRadonClient(endpoint string) *radonClient {
	return &radonClient{http: &http.Client{Timeout: radonRequestTimeout}, endpoint: endpoint}
}

type radonPage struct {
	Results    []radonEmployee `json:"results"`
	Pagination struct {
		MaxCount int    `json:"maxCount"`
		Token    string `json:"token"`
	} `json:"pagination"`
}

type radonEmployee struct {
	ID                 string `json:"id"`
	CalculatedEduLevel string `json:"calculatedEduLevel"`
	PersonalData       struct {
		FirstName  string `json:"firstName"`
		MiddleName string `json:"middleName"`
		LastName   string `json:"lastName"`
	} `json:"personalData"`
	Posts []struct {
		EmployeeID    uuid.UUID `json:"employeeInInstitutionUuid"`
		InstitutionID uuid.UUID `json:"employingInstitutionUuid"`
	} `json:"employeeInInstitutionUuids"`
}

func (c *radonClient) employees(ctx context.Context, institutionID uuid.UUID) ([]radonEmployee, error) {
	var all []radonEmployee
	token := ""
	for range radonMaxPages {
		page, err := c.page(ctx, institutionID, token)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Results...)
		next := page.Pagination.Token
		if len(page.Results) == 0 || next == "" || next == token || len(all) >= page.Pagination.MaxCount {
			return all, nil
		}
		token = next
	}
	return nil, fmt.Errorf("radon employees of %s: more than %d pages", institutionID, radonMaxPages)
}

func (c *radonClient) page(ctx context.Context, institutionID uuid.UUID, token string) (radonPage, error) {
	query := url.Values{}
	query.Set("employingInstitutionUuid", institutionID.String())
	query.Set("resultNumbers", strconv.Itoa(radonPageSize))
	if token != "" {
		query.Set("token", token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return radonPage{}, fmt.Errorf("build radon request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return radonPage{}, fmt.Errorf("get radon employees: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return radonPage{}, fmt.Errorf("get radon employees: status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, radonMaxPageBytes+1))
	if err != nil {
		return radonPage{}, fmt.Errorf("read radon employees: %w", err)
	}
	if len(payload) > radonMaxPageBytes {
		return radonPage{}, fmt.Errorf("read radon employees: page over %d bytes", radonMaxPageBytes)
	}
	var page radonPage
	if err := json.Unmarshal(payload, &page); err != nil {
		return radonPage{}, fmt.Errorf("decode radon employees: %w", err)
	}
	return page, nil
}
