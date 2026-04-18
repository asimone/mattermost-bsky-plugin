package main

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Configuration holds plugin settings from the System Console.
type Configuration struct {
	ShowEngagementCounts bool `json:"ShowEngagementCounts"`
}

// getConfiguration loads the current config, defaulting engagement counts to on.
// Setting the default before LoadPluginConfiguration ensures "never saved" == true.
func (p *Plugin) getConfiguration() Configuration {
	cfg := Configuration{ShowEngagementCounts: true}
	_ = p.API.LoadPluginConfiguration(&cfg)
	return cfg
}

// Plugin implements the Mattermost plugin interface.
type Plugin struct {
	plugin.MattermostPlugin
	bsky *BlueskyClient
}

func (p *Plugin) OnActivate() error {
	p.bsky = NewBlueskyClient()
	return nil
}

// MessageWillBePosted intercepts posts before they are saved. It detects bsky.app
// URLs and appends rich SlackAttachments fetched from the public AT Protocol API.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	// Skip system/bot messages.
	if post.Type != "" {
		return post, ""
	}

	urls := extractBlueskyURLs(post.Message)
	if len(urls) == 0 {
		return post, ""
	}

	seen := make(map[string]bool)
	var attachments []*model.SlackAttachment

	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true

		// Cap at 3 unfurls per message to avoid excessive API calls.
		if len(attachments) >= 3 {
			break
		}

		cfg := p.getConfiguration()
		atts, err := p.bsky.UnfurlURL(u, cfg.ShowEngagementCounts)
		if err != nil {
			p.API.LogWarn("bluesky unfurl failed", "url", u, "err", err.Error())
			continue
		}
		attachments = append(attachments, atts...)
	}

	if len(attachments) == 0 {
		return post, ""
	}

	if post.Props == nil {
		post.Props = make(model.StringInterface)
	}
	post.Props["attachments"] = attachments
	return post, ""
}

func main() {
	plugin.ClientMain(&Plugin{})
}
