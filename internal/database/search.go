package database

func SearchPattern(param string) string {
	normalized := "public.search_text(" + param + ")"
	return "('%' || replace(replace(replace(" + normalized + ", '!', '!!'), '%', '!%'), '_', '!_') || '%') ESCAPE '!'"
}
