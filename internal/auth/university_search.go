package auth

func MatchingUniversities(pattern string) string {
	return `SELECT id FROM supported_universities
		WHERE public.search_text(id) LIKE ` + pattern + ` OR public.search_text(name) LIKE ` + pattern
}
