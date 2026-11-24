package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	expoPushURL         = "https://exp.host/--/api/v2/push/send"
	expoBatch           = 100
	deviceNotRegistered = "DeviceNotRegistered"
)

type expoMessage struct {
	To        string            `json:"to"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Data      map[string]string `json:"data"`
	Sound     string            `json:"sound"`
	ChannelID string            `json:"channelId"`
}

type expoTicket struct {
	Status  string `json:"status"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoClient struct {
	http        *http.Client
	url         string
	accessToken string
}

func (c *expoClient) send(ctx context.Context, messages []expoMessage) ([]string, error) {
	var invalid []string
	for start := 0; start < len(messages); start += expoBatch {
		batch := messages[start:min(start+expoBatch, len(messages))]
		tickets, err := c.post(ctx, batch)
		if err != nil {
			return invalid, err
		}
		for i, ticket := range tickets {
			if i < len(batch) && ticket.Status == "error" && ticket.Details.Error == deviceNotRegistered {
				invalid = append(invalid, batch[i].To)
			}
		}
	}
	return invalid, nil
}

func (c *expoClient) post(ctx context.Context, batch []expoMessage) ([]expoTicket, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("encode push messages: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send push messages: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("send push messages: expo answered %d", resp.StatusCode)
	}

	var out struct {
		Data []expoTicket `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode push tickets: %w", err)
	}
	return out.Data, nil
}
