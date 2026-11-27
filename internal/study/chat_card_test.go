package study

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestChatCards(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a course gives its name and faculty", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)

		cards, err := e.module.CourseCards(ctx, []uuid.UUID{course.ID, uuid.New()})
		if err != nil {
			t.Fatalf("CourseCards: %v", err)
		}
		if card, ok := cards[course.ID]; len(cards) != 1 || !ok || card.Title != course.Name || card.Subtitle != course.Faculty {
			t.Errorf("cards = %+v", cards)
		}
	})

	t.Run("notes give their title and course, and are left out when their author is blocked", func(t *testing.T) {
		author, viewer := e.student(t), e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Exam 2025")

		cards, err := e.module.NotesCards(ctx, viewer.id, []uuid.UUID{file.ID})
		if err != nil {
			t.Fatalf("NotesCards: %v", err)
		}
		if card := cards[file.ID]; card.Title != "Exam 2025" || card.Subtitle != course.Name {
			t.Errorf("cards = %+v", cards)
		}

		e.block(t, viewer.id, author.id)
		cards, err = e.module.NotesCards(ctx, viewer.id, []uuid.UUID{file.ID})
		if err != nil || len(cards) != 0 {
			t.Errorf("after block = %v, %v", cards, err)
		}
	})

	t.Run("an empty list returns no cards", func(t *testing.T) {
		cards, err := e.module.NotesCards(ctx, uuid.New(), nil)
		if err != nil || len(cards) != 0 {
			t.Errorf("cards = %v, %v", cards, err)
		}
	})
}
