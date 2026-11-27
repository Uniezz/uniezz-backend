-- +goose Up
-- An entry holds its text in every interface language. `language` is the one its author wrote in.
ALTER TABLE faq_entries
    ADD COLUMN language TEXT NOT NULL DEFAULT 'pl'
        CHECK (language IN ('en', 'pl', 'uk', 'ru', 'be', 'de', 'es')),
    ADD COLUMN texts JSONB;

-- Clients never saved the interface language before this, so every existing entry counts as Polish.
UPDATE faq_entries
SET texts = jsonb_build_object('pl', jsonb_build_object('question', question, 'answer', answer));

ALTER TABLE faq_entries
    ALTER COLUMN texts SET NOT NULL,
    ADD CONSTRAINT faq_entries_texts_has_language CHECK (texts ? language),
    DROP COLUMN question,
    DROP COLUMN answer;

-- +goose Down
ALTER TABLE faq_entries
    ADD COLUMN question VARCHAR(200),
    ADD COLUMN answer TEXT;

UPDATE faq_entries
SET question = texts -> language ->> 'question',
    answer = texts -> language ->> 'answer';

ALTER TABLE faq_entries
    ALTER COLUMN question SET NOT NULL,
    ALTER COLUMN answer SET NOT NULL,
    ADD CONSTRAINT faq_entries_question_check CHECK (char_length(question) BETWEEN 3 AND 200),
    ADD CONSTRAINT faq_entries_answer_check CHECK (char_length(answer) BETWEEN 1 AND 2000),
    DROP COLUMN texts,
    DROP COLUMN language;
