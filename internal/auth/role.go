package auth

import (
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

type Role string

const (
	RoleModerator Role = "moderator"
	RoleAdmin     Role = "admin"
)

func (Role) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{string(RoleModerator), string(RoleAdmin)}
	return schema
}

func (p principal) authorize(required []string) error {
	if len(required) == 0 {
		return nil
	}
	for _, role := range p.roles {
		if slices.Contains(required, string(role)) {
			return nil
		}
	}
	return ErrForbidden
}
