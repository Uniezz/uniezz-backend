package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/safehttp"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
	"golang.org/x/net/html"
)

const (
	previewFreshFor   = 30 * 24 * time.Hour
	previewRetryAfter = 24 * time.Hour
	pageFetchTimeout  = 3 * time.Second
	maxPageBytes      = 512 << 10
)

var pageTypes = []string{"text/html", "application/xhtml+xml"}

var httpsLinkPattern = regexp.MustCompile(`(?i)https://[^\s<>"]+`)

type messageLink struct {
	URL  string
	Hash []byte
}

func firstLink(body string) *messageLink {
	raw := httpsLinkPattern.FindString(body)
	raw = strings.TrimRight(raw, ".,;:!?)]}'")
	if raw == "" || len(raw) > maxLinkLength {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return nil
	}
	u.Scheme = "https"
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	normalized := u.String()
	if len(normalized) > maxLinkLength {
		return nil
	}
	hash := sha256.Sum256([]byte(normalized))
	return &messageLink{URL: normalized, Hash: hash[:]}
}

type pageMeta struct {
	Title       string
	Description string
	SiteName    string
	Image       string
}

func parseMeta(body []byte) pageMeta {
	values := map[string]string{}
	var title strings.Builder
	inTitle := false
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return metaOf(values, title.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "body":
				return metaOf(values, title.String())
			case "title":
				inTitle = true
			case "meta":
				var key, content string
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					switch strings.ToLower(string(k)) {
					case "property", "name":
						key = strings.ToLower(string(v))
					case "content":
						content = string(v)
					}
				}
				if _, seen := values[key]; key != "" && !seen {
					values[key] = content
				}
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "title" {
				inTitle = false
			}
		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		}
	}
}

func firstValue(values map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(values[k]); v != "" {
			return v
		}
	}
	return ""
}

func metaOf(values map[string]string, title string) pageMeta {
	meta := pageMeta{
		Title:       firstValue(values, "og:title", "twitter:title"),
		Description: firstValue(values, "og:description", "twitter:description", "description"),
		SiteName:    firstValue(values, "og:site_name"),
		Image:       firstValue(values, "og:image", "og:image:url", "twitter:image"),
	}
	if meta.Title == "" {
		meta.Title = strings.TrimSpace(title)
	}
	return meta
}

func clip(s string, limit int) *string {
	clean := text.Plain(s, false)
	if clean == "" {
		return nil
	}
	if utf8.RuneCountInString(clean) > limit {
		clean = strings.TrimSpace(string([]rune(clean)[:limit-1])) + "…"
	}
	return &clean
}

func imageURL(page *url.URL, raw string) string {
	if raw == "" {
		return ""
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u := page.ResolveReference(ref)
	if u.Scheme != "https" || u.Hostname() == "" || len(u.String()) > maxLinkLength {
		return ""
	}
	return u.String()
}

func (m Module) queuePreview(ctx context.Context, messageID uuid.UUID, link *messageLink) error {
	if link == nil {
		return nil
	}
	if err := m.previews.createLink(ctx, messageID, link.Hash, link.URL); err != nil {
		return err
	}
	fresh, err := m.previews.fresh(ctx, link.Hash, previewFreshFor, previewRetryAfter)
	if err != nil || fresh {
		return err
	}
	return m.jobs.Enqueue(ctx, FetchLinkPreviewArgs{MessageID: messageID})
}

func (m Module) fetchPreview(ctx context.Context, link *messageLinkRow) (previewFields, error) {
	fields := previewFields{URLHash: link.URLHash, URL: link.URL, Status: previewFailed}
	resp, err := m.pages.Get(ctx, link.URL, safehttp.Limits{Timeout: pageFetchTimeout, MaxBytes: maxPageBytes, ContentTypes: pageTypes})
	if err != nil {
		if errors.Is(err, safehttp.ErrRefused) || errors.Is(err, safehttp.ErrTooLarge) ||
			errors.Is(err, safehttp.ErrUnsupportedType) || errors.Is(err, safehttp.ErrStatus) {
			return fields, nil
		}
		return fields, err
	}

	meta := parseMeta(resp.Body)
	fields.Title = clip(meta.Title, maxPreviewTitle)
	fields.Description = clip(meta.Description, maxPreviewDescription)
	fields.SiteName = clip(meta.SiteName, maxPreviewSiteName)
	if fields.Title == nil && fields.Description == nil {
		return fields, nil
	}
	fields.Status = previewOK

	if image := imageURL(resp.URL, meta.Image); image != "" && link.SenderID != nil {
		id, err := m.media.Import(ctx, *link.SenderID, media.KindLinkPreview, image)
		if err != nil {
			slog.WarnContext(ctx, "import link preview image", slog.String("message_id", link.MessageID.String()), slog.Any("error", err))
		} else {
			fields.MediaID = &id
		}
	}
	return fields, nil
}

func (m Module) storePreview(ctx context.Context, link *messageLinkRow, fields previewFields) ([]notification, error) {
	if fields.Status != previewOK {
		return nil, m.previews.markFailed(ctx, fields)
	}

	var notes []notification
	var replaced *uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		previous, err := m.previews.replace(ctx, fields)
		if err != nil {
			return err
		}
		if previous != nil && (fields.MediaID == nil || *previous != *fields.MediaID) {
			replaced = previous
			if err := m.media.MarkDeletingMany(ctx, []uuid.UUID{*previous}); err != nil {
				return err
			}
		}
		notes, err = m.touchMessage(ctx, link.MessageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if replaced != nil {
		m.releaseMedia(ctx, []uuid.UUID{*replaced})
	}
	return notes, nil
}

func (m Module) announcePreview(ctx context.Context, messageID uuid.UUID) ([]notification, error) {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		notes, err = m.touchMessage(ctx, messageID)
		return err
	})
	return notes, err
}

func (m Module) touchMessage(ctx context.Context, messageID uuid.UUID) ([]notification, error) {
	row, err := m.messages.get(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if row.DeletedAt != nil {
		return nil, nil
	}
	changeSeq, err := m.conversations.bumpChange(ctx, row.ConversationID)
	if err != nil {
		return nil, err
	}
	if err := m.messages.setChange(ctx, messageID, changeSeq); err != nil {
		return nil, err
	}
	row.ChangeSeq = changeSeq
	actorID := uuid.Nil
	if row.SenderID != nil {
		actorID = *row.SenderID
	}
	recipients, err := m.members.recipients(ctx, row.ConversationID, actorID)
	if err != nil {
		return nil, err
	}
	return []notification{{Event: eventMessageUpdated, Recipients: recipients, ConversationID: row.ConversationID, ChangeSeq: changeSeq, Message: row}}, nil
}

func (m Module) previewViews(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]*LinkPreview, error) {
	views := map[uuid.UUID]*LinkPreview{}
	if len(messageIDs) == 0 {
		return views, nil
	}
	rows, err := m.previews.previews(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range rows {
		views[p.MessageID] = &LinkPreview{URL: p.URL, Title: p.Title, Description: p.Description, SiteName: p.SiteName, ImageURL: m.mediaURL(ctx, p.Image)}
	}
	return views, nil
}
