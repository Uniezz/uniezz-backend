package study

const (
	minRating = 1
	maxRating = 5
)

func validRating(rating int) bool {
	return rating >= minRating && rating <= maxRating
}

func ratingAverage(sum, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}
