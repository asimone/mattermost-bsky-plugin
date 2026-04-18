package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// bskyURLRe matches https://bsky.app/profile/<handle-or-did>/post/<rkey>
var bskyURLRe = regexp.MustCompile(
	`https://bsky\.app/profile/([A-Za-z0-9._:%-]+)/post/([A-Za-z0-9]+)`,
)

// mdLinkRe matches a standalone markdown link token [text](url).
var mdLinkRe = regexp.MustCompile(`^\[([^\]]*)\]\([^)]*\)$`)

const wrapWidth = 90

func extractBlueskyURLs(text string) []string {
	matches := bskyURLRe.FindAllString(text, -1)
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func parseBlueskyURL(rawURL string) (handle, rkey string, err error) {
	sub := bskyURLRe.FindStringSubmatch(rawURL)
	if sub == nil {
		return "", "", fmt.Errorf("not a recognised Bluesky post URL: %s", rawURL)
	}
	return sub[1], sub[2], nil
}

// UnfurlURL resolves a bsky.app URL to one or two SlackAttachments.
// When the post contains a quote embed a second attachment is returned for the quoted post.
func (c *BlueskyClient) UnfurlURL(rawURL string, showEngagement bool) ([]*model.SlackAttachment, error) {
	handle, rkey, err := parseBlueskyURL(rawURL)
	if err != nil {
		return nil, err
	}

	did := handle
	if !strings.HasPrefix(handle, "did:") {
		did, err = c.resolveHandle(handle)
		if err != nil {
			return nil, fmt.Errorf("resolve handle %q: %w", handle, err)
		}
	}

	post, err := c.fetchPost(did, rkey)
	if err != nil {
		return nil, fmt.Errorf("fetch post: %w", err)
	}

	return buildAttachments(post, showEngagement), nil
}

// buildAttachments returns the outer post attachment and, when a quote is embedded,
// a second attachment for the quoted post.
func buildAttachments(post *PostView, showEngagement bool) []*model.SlackAttachment {
	var rec PostRecord
	_ = json.Unmarshal(post.Record, &rec)

	authorDisplay := post.Author.DisplayName
	if authorDisplay == "" {
		authorDisplay = post.Author.Handle
	}
	authorName := fmt.Sprintf("%s (@%s)", authorDisplay, post.Author.Handle)

	att := &model.SlackAttachment{
		Color:      "#0085ff",
		Fallback:   fmt.Sprintf("%s: %s", authorName, rec.Text),
		AuthorName: authorName,
		AuthorLink: fmt.Sprintf("https://bsky.app/profile/%s", post.Author.Handle),
		AuthorIcon: post.Author.Avatar,
		Text:       wrapText(applyFacets(rec.Text, rec.Facets)),
	}

	var quote *QuoteRecord
	if post.Embed != nil {
		quote = applyEmbed(att, post.Embed)
	}

	att.Text += inlineFooter(rec.CreatedAt)

	if showEngagement {
		att.Fields = engagementFields(post.ReplyCount, post.RepostCount, post.LikeCount, post.QuoteCount)
	}

	attachments := []*model.SlackAttachment{att}

	if quote != nil {
		attachments = append(attachments, buildQuoteAttachment(quote, showEngagement))
	}

	return attachments
}

// buildQuoteAttachment builds a full attachment for a quoted post, including its
// own embeds (e.g. images in the quoted post) and engagement counts.
func buildQuoteAttachment(q *QuoteRecord, showEngagement bool) *model.SlackAttachment {
	authorDisplay := q.Author.DisplayName
	if authorDisplay == "" {
		authorDisplay = q.Author.Handle
	}
	authorName := fmt.Sprintf("%s (@%s)", authorDisplay, q.Author.Handle)

	att := &model.SlackAttachment{
		Color:      "#004f99",
		Fallback:   fmt.Sprintf("%s: %s", authorName, q.Value.Text),
		AuthorName: authorName,
		AuthorLink: fmt.Sprintf("https://bsky.app/profile/%s", q.Author.Handle),
		AuthorIcon: q.Author.Avatar,
		Title:      "Quoted post",
		TitleLink:  atURIToPostURL(q.URI, q.Author.Handle),
		Text:       wrapText(applyFacets(q.Value.Text, q.Value.Facets)),
	}

	// The quoted post may itself have image/external embeds; apply them.
	// Nested quotes are not expanded to avoid infinite recursion.
	for i := range q.Embeds {
		applyEmbed(att, &q.Embeds[i])
	}

	att.Text += inlineFooter(q.Value.CreatedAt)

	if showEngagement {
		att.Fields = engagementFields(q.ReplyCount, q.RepostCount, q.LikeCount, q.QuoteCount)
	}

	return att
}

// applyEmbed mutates att for images and external cards.
// For quote embeds it returns the QuoteRecord so the caller can build a separate attachment.
func applyEmbed(att *model.SlackAttachment, embed *EmbedView) *QuoteRecord {
	switch embed.Type {
	case "app.bsky.embed.images#view":
		applyImages(att, embed.Images)

	case "app.bsky.embed.external#view":
		applyExternal(att, embed.External)

	case "app.bsky.embed.record#view":
		return extractQuote(embed.Record)

	case "app.bsky.embed.recordWithMedia#view":
		var mediaEmbed EmbedView
		if len(embed.Media) > 0 {
			if err := json.Unmarshal(embed.Media, &mediaEmbed); err == nil {
				applyEmbed(att, &mediaEmbed)
			}
		}
		return extractQuote(embed.Record)
	}
	return nil
}

func extractQuote(rawRecord json.RawMessage) *QuoteRecord {
	if len(rawRecord) == 0 {
		return nil
	}
	var q QuoteRecord
	if err := json.Unmarshal(rawRecord, &q); err != nil {
		return nil
	}
	if q.Type != "app.bsky.embed.record#viewRecord" {
		return nil
	}
	return &q
}

func applyImages(att *model.SlackAttachment, images []ImageView) {
	if len(images) == 0 {
		return
	}
	att.ImageURL = images[0].Thumb
	if len(images) > 1 {
		att.Text += fmt.Sprintf("\n\n_%d images attached_", len(images))
	}
}

func applyExternal(att *model.SlackAttachment, ext *ExternalView) {
	if ext == nil {
		return
	}
	att.Title = ext.Title
	att.TitleLink = ext.URI
	if ext.Description != "" {
		att.Text += "\n\n> " + ext.Description
	}
	if ext.Thumb != "" && att.ImageURL == "" {
		att.ThumbURL = ext.Thumb
	}
}

// inlineFooter returns the Bluesky icon + date line appended to Text so it sits
// below the post body but above the ImageURL in Mattermost's attachment renderer.
func inlineFooter(createdAt string) string {
	footer := "\n\n![](https://bsky.app/static/favicon-16x16.png) Bluesky"
	if date := formatPostDate(createdAt); date != "" {
		footer += " | " + date
	}
	return footer
}

func engagementFields(replies, reposts, likes, quotes int) []*model.SlackAttachmentField {
	fields := []*model.SlackAttachmentField{
		{Title: "Replies", Value: fmt.Sprintf("%d", replies), Short: true},
		{Title: "Reposts", Value: fmt.Sprintf("%d", reposts), Short: true},
		{Title: "Likes", Value: fmt.Sprintf("%d", likes), Short: true},
	}
	if quotes > 0 {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Quotes", Value: fmt.Sprintf("%d", quotes), Short: true,
		})
	}
	return fields
}

// atURIToPostURL converts an AT URI to a bsky.app post URL using the known handle.
func atURIToPostURL(atURI, handle string) string {
	// at://did/app.bsky.feed.post/rkey -> https://bsky.app/profile/handle/post/rkey
	parts := strings.Split(atURI, "/")
	if len(parts) >= 5 {
		return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, parts[len(parts)-1])
	}
	return fmt.Sprintf("https://bsky.app/profile/%s", handle)
}

// wrapText word-wraps text at wrapWidth display characters, respecting existing newlines
// and treating markdown links [text](url) as len(text) wide (not len(raw markdown) wide).
func wrapText(text string) string {
	var out strings.Builder
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(wrapLine(line))
	}
	return out.String()
}

func wrapLine(line string) string {
	words := strings.Split(line, " ")
	var out strings.Builder
	col := 0
	for i, w := range words {
		dw := wordDisplayLen(w)
		if i > 0 {
			if col > 0 && col+1+dw > wrapWidth {
				out.WriteByte('\n')
				col = 0
			} else {
				out.WriteByte(' ')
				col++
			}
		}
		out.WriteString(w)
		col += dw
	}
	return out.String()
}

// wordDisplayLen returns the visible character width of a single space-separated token,
// collapsing markdown link markup [text](url) down to len(text).
func wordDisplayLen(word string) int {
	if m := mdLinkRe.FindStringSubmatch(word); m != nil {
		return len([]rune(m[1]))
	}
	return len([]rune(word))
}

// applyFacets rewrites text by replacing link byte-ranges with Markdown [text](url).
// Indices in Bluesky facets are UTF-8 byte offsets, so we operate on []byte throughout.
func applyFacets(text string, facets []Facet) string {
	if len(facets) == 0 {
		return text
	}

	sort.Slice(facets, func(i, j int) bool {
		return facets[i].Index.ByteStart < facets[j].Index.ByteStart
	})

	b := []byte(text)
	var out strings.Builder
	cursor := 0

	for _, facet := range facets {
		s, e := facet.Index.ByteStart, facet.Index.ByteEnd
		if s < cursor || e > len(b) || s >= e {
			continue
		}

		var linkURI string
		for _, feat := range facet.Features {
			if feat.Type == "app.bsky.richtext.facet#link" && feat.URI != "" {
				linkURI = feat.URI
				break
			}
		}

		out.Write(b[cursor:s])
		if linkURI != "" {
			out.WriteByte('[')
			out.Write(b[s:e])
			out.WriteString("](")
			out.WriteString(linkURI)
			out.WriteByte(')')
		} else {
			out.Write(b[s:e])
		}
		cursor = e
	}

	out.Write(b[cursor:])
	return out.String()
}

func formatPostDate(createdAt string) string {
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return ""
		}
	}
	return t.Format("Jan 2, 2006")
}
