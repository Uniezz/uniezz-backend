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
	radonURL            = "https://radon.nauka.gov.pl/opendata/polon"
	radonRequestTimeout = 20 * time.Second
	radonPageSize       = 100
	radonMaxPages       = 200
	radonMaxPageBytes   = 8 << 20
)

type radonClient struct {
	http    *http.Client
	baseURL string
}

func newRadonClient(baseURL string) *radonClient {
	return &radonClient{http: &http.Client{Timeout: radonRequestTimeout}, baseURL: baseURL}
}

type radonPage[T any] struct {
	Results    []T `json:"results"`
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

type radonCourse struct {
	ID                string `json:"courseUuid"`
	Name              string `json:"courseName"`
	LevelCode         string `json:"levelCode"`
	StatusCode        string `json:"currentStatusCode"`
	OrganizationUnits []struct {
		FullName string `json:"organizationalUnitFullName"`
	} `json:"organizationalUnits"`
}

func (c *radonClient) employees(ctx context.Context, institutionID uuid.UUID) ([]radonEmployee, error) {
	return radonList[radonEmployee](ctx, c, "employees", "employingInstitutionUuid", institutionID)
}

func (c *radonClient) courses(ctx context.Context, institutionID uuid.UUID) ([]radonCourse, error) {
	return radonList[radonCourse](ctx, c, "courses", "leadingInstitutionUuid", institutionID)
}

func radonList[T any](ctx context.Context, c *radonClient, collection, institutionParam string, institutionID uuid.UUID) ([]T, error) {
	var all []T
	token := ""
	for range radonMaxPages {
		page, err := radonGet[T](ctx, c, collection, institutionParam, institutionID, token)
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
	return nil, fmt.Errorf("radon %s of %s: more than %d pages", collection, institutionID, radonMaxPages)
}

func radonGet[T any](ctx context.Context, c *radonClient, collection, institutionParam string, institutionID uuid.UUID, token string) (radonPage[T], error) {
	query := url.Values{}
	query.Set(institutionParam, institutionID.String())
	query.Set("resultNumbers", strconv.Itoa(radonPageSize))
	if token != "" {
		query.Set("token", token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/"+collection+"?"+query.Encode(), nil)
	if err != nil {
		return radonPage[T]{}, fmt.Errorf("build radon request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return radonPage[T]{}, fmt.Errorf("get radon %s: %w", collection, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return radonPage[T]{}, fmt.Errorf("get radon %s: status %d", collection, resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, radonMaxPageBytes+1))
	if err != nil {
		return radonPage[T]{}, fmt.Errorf("read radon %s: %w", collection, err)
	}
	if len(payload) > radonMaxPageBytes {
		return radonPage[T]{}, fmt.Errorf("read radon %s: page over %d bytes", collection, radonMaxPageBytes)
	}
	var page radonPage[T]
	if err := json.Unmarshal(payload, &page); err != nil {
		return radonPage[T]{}, fmt.Errorf("decode radon %s: %w", collection, err)
	}
	return page, nil
}
