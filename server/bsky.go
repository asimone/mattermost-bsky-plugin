package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const bskyAPI = "https://public.api.bsky.app/xrpc"

// BlueskyClient calls the public AT Protocol API without authentication.
type BlueskyClient struct {
	http *http.Client
}

func NewBlueskyClient() *BlueskyClient {
	return &BlueskyClient{
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// --- API response types ---

type threadResponse struct {
	Thread struct {
		Type string   `json:"$type"`
		Post PostView `json:"post"`
	} `json:"thread"`
}

// PostView is the post as returned by app.bsky.feed.getPostThread.
type PostView struct {
	URI         string          `json:"uri"`
	Author      Author          `json:"author"`
	Record      json.RawMessage `json:"record"` // app.bsky.feed.post record
	Embed       *EmbedView      `json:"embed,omitempty"`
	ReplyCount  int             `json:"replyCount"`
	RepostCount int             `json:"repostCount"`
	LikeCount   int             `json:"likeCount"`
	QuoteCount  int             `json:"quoteCount"`
}

type Author struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

// PostRecord is the lexicon record stored in the repository.
type PostRecord struct {
	Text      string  `json:"text"`
	CreatedAt string  `json:"createdAt"`
	Facets    []Facet `json:"facets,omitempty"`
}

// Facet marks a byte range in post text with a feature (link, mention, hashtag).
// Indices are UTF-8 byte offsets, not character positions.
type Facet struct {
	Index    FacetIndex     `json:"index"`
	Features []FacetFeature `json:"features"`
}

type FacetIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type FacetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri,omitempty"` // app.bsky.richtext.facet#link
	DID  string `json:"did,omitempty"` // app.bsky.richtext.facet#mention
	Tag  string `json:"tag,omitempty"` // app.bsky.richtext.facet#tag
}

// EmbedView is the rendered view of an embed returned by the AppView.
// The $type discriminator determines which fields are populated.
type EmbedView struct {
	Type string `json:"$type"`
	// app.bsky.embed.images#view
	Images []ImageView `json:"images,omitempty"`
	// app.bsky.embed.external#view
	External *ExternalView `json:"external,omitempty"`
	// app.bsky.embed.record#view  (quote post)
	Record json.RawMessage `json:"record,omitempty"`
	// app.bsky.embed.recordWithMedia#view — media sub-embed
	Media json.RawMessage `json:"media,omitempty"`
}

type ImageView struct {
	Thumb    string `json:"thumb"`
	Fullsize string `json:"fullsize"`
	Alt      string `json:"alt"`
}

type ExternalView struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Thumb       string `json:"thumb"`
}

// QuoteRecord is an app.bsky.embed.record#viewRecord inside a quote embed.
type QuoteRecord struct {
	Type        string      `json:"$type"`
	URI         string      `json:"uri"`
	Author      Author      `json:"author"`
	Value       QuoteValue  `json:"value"`
	Embeds      []EmbedView `json:"embeds,omitempty"` // images/external on the quoted post
	LikeCount   int         `json:"likeCount"`
	RepostCount int         `json:"repostCount"`
	ReplyCount  int         `json:"replyCount"`
	QuoteCount  int         `json:"quoteCount"`
}

type QuoteValue struct {
	Text      string  `json:"text"`
	CreatedAt string  `json:"createdAt"`
	Facets    []Facet `json:"facets,omitempty"`
}

// --- API methods ---

// resolveHandle converts a handle (e.g. "alice.bsky.social") to a DID.
func (c *BlueskyClient) resolveHandle(handle string) (string, error) {
	u := fmt.Sprintf("%s/com.atproto.identity.resolveHandle?handle=%s", bskyAPI, url.QueryEscape(handle))
	resp, err := c.http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolveHandle HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var r struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.DID == "" {
		return "", fmt.Errorf("empty DID for handle %q", handle)
	}
	return r.DID, nil
}

// fetchPost retrieves a PostView from the public AppView.
func (c *BlueskyClient) fetchPost(did, rkey string) (*PostView, error) {
	atURI := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey)
	u := fmt.Sprintf("%s/app.bsky.feed.getPostThread?uri=%s&depth=0&parentHeight=0",
		bskyAPI, url.QueryEscape(atURI))

	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getPostThread HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var t threadResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, err
	}
	return &t.Thread.Post, nil
}
