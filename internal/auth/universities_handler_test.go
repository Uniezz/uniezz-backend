package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

func TestUniversitiesHandler_ReturnsFacultiesWithLocations(t *testing.T) {
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{BackendPublicURL: "https://api.test"}))
	Module{}.Register(api)

	resp := api.Get("/auth/universities")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var body []UniversityDTO
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	domain := GetUniversities()
	if len(body) != len(domain) {
		t.Fatalf("expected %d universities, got %d", len(domain), len(body))
	}

	for i, u := range body {
		want := domain[i]
		if len(u.Faculties) != len(want.Faculties) {
			t.Errorf("%s: expected %d faculties, got %d", u.ID, len(want.Faculties), len(u.Faculties))
			continue
		}
		for j, f := range u.Faculties {
			w := want.Faculties[j]
			if f.Name != w.Name || f.Address != w.Address ||
				f.Location.Latitude != w.Location.Latitude || f.Location.Longitude != w.Location.Longitude {
				t.Errorf("%s: faculty %d = %+v, want %+v", u.ID, j, f, w)
			}
		}
	}
}

func TestUniversitiesHandler_LocationJSONShape(t *testing.T) {
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{BackendPublicURL: "https://api.test"}))
	Module{}.Register(api)

	resp := api.Get("/auth/universities")

	var body []map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	faculties, ok := body[0]["faculties"].([]any)
	if !ok || len(faculties) == 0 {
		t.Fatalf("expected non-empty faculties array, got %v", body[0]["faculties"])
	}
	faculty, ok := faculties[0].(map[string]any)
	if !ok {
		t.Fatalf("expected faculty object, got %v", faculties[0])
	}
	location, ok := faculty["location"].(map[string]any)
	if !ok {
		t.Fatalf("expected faculty location object, got %v", faculty["location"])
	}
	if _, ok := location["latitude"].(float64); !ok {
		t.Errorf("expected numeric latitude, got %v", location["latitude"])
	}
	if _, ok := location["longitude"].(float64); !ok {
		t.Errorf("expected numeric longitude, got %v", location["longitude"])
	}
}
