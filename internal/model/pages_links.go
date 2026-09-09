package model

// PagesLinkType describes a supported link and its icon.
type PagesLinkType struct {
	Key, Label, Icon string
}

// PagesLinkTypes lists the supported links in default order.
var PagesLinkTypes = []PagesLinkType{
	{"website", "Website", "globe-24"},
	{"x", "X", "x"},
	{"bluesky", "Bluesky", "bluesky"},
	{"mastodon", "Mastodon", "mastodon"},
	{"discord", "Discord", "discord"},
	{"matrix", "Matrix", "matrix"},
	{"slack", "Slack", "organization-24"},
	{"linkedin", "LinkedIn", "briefcase-24"},
	{"youtube", "YouTube", "youtube"},
	{"instagram", "Instagram", "instagram"},
	{"pixelfed", "Pixelfed", "pixelfed"},
	{"tiktok", "TikTok", "tiktok"},
	{"peertube", "PeerTube", "peertube"},
	{"steam", "Steam", "steam"},
	{"itchio", "itch.io", "itchdotio"},
	{"patreon", "Patreon", "patreon"},
	{"kofi", "Ko-fi", "kofi"},
	{"forum", "Forum", "comment-discussion-24"},
	{"email", "Email", "mail-24"},
	{"feed", "RSS or Atom feed", "rss-24"},
}

// OrderedLinks returns declared links in YAML order, with defaults for new keys.
func (p *PagesSettings) OrderedLinks() []PagesLinkType {
	if p == nil || p.Links == nil {
		return nil
	}
	var links []PagesLinkType
	seen := make(map[string]bool)
	appendLink := func(link PagesLinkType) {
		if _, ok := (*p.Links)[link.Key]; ok && !seen[link.Key] {
			links = append(links, link)
			seen[link.Key] = true
		}
	}
	for _, key := range p.linksOrder {
		for _, link := range PagesLinkTypes {
			if link.Key == key {
				appendLink(link)
				break
			}
		}
	}
	for _, link := range PagesLinkTypes {
		appendLink(link)
	}
	return links
}
