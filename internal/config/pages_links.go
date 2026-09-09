package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/wimpysworld/tailor/internal/model"
)

func validatePagesLinks(p *model.PagesSettings) error {
	if p.Links == nil {
		return nil
	}
	keys := make([]string, 0, len(model.PagesLinkTypes))
	for _, link := range model.PagesLinkTypes {
		keys = append(keys, link.Key)
	}
	for key, value := range *p.Links {
		if !slices.Contains(keys, key) {
			return fmt.Errorf("unknown pages.links key %q", key)
		}
		if value == "" {
			continue
		}
		if p.Generator != nil && *p.Generator != "static" {
			return fmt.Errorf("non-empty pages.links requires generator static")
		}
		if len(value) > 2048 || strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
			return fmt.Errorf("pages.links.%s must not exceed 2048 bytes or contain whitespace or control characters", key)
		}
		if key == "email" {
			address, err := mail.ParseAddress(value)
			if err != nil || address.Address != value || strings.ContainsAny(value, "?&#%") {
				return fmt.Errorf("pages.links.email must be a bare email address without mail headers")
			}
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.Contains(value, "\\") {
			return fmt.Errorf("pages.links.%s must be a full HTTPS URL without credentials", key)
		}
	}
	return nil
}
