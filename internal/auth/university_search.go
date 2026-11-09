package auth

import "github.com/Uniezz/uniezz-backend/internal/database"

func MatchingUniversities(args *database.Args, pattern string) string {
	return `SELECT id FROM unnest(` + args.Add(universityIDs) + `::text[], ` + args.Add(universityNames) + `::text[]) AS universities(id, name)
		WHERE public.search_text(id) LIKE ` + pattern + ` OR public.search_text(name) LIKE ` + pattern
}
