package cache

import (
	"testing"

	"github.com/luuuunet/owpanel/internal/models"
)

func TestStaticTTLNilConfig(t *testing.T) {
	s := &Service{}
	site := &models.Website{CacheEnabled: true}
	if got := s.staticTTL(site, nil); got != 168 {
		t.Fatalf("staticTTL(site, nil) = %d, want 168", got)
	}
}

func TestHtmlTTLNilConfig(t *testing.T) {
	s := &Service{}
	site := &models.Website{CacheEnabled: true}
	if got := s.htmlTTL(site, nil); got != 5 {
		t.Fatalf("htmlTTL(site, nil) = %d, want 5", got)
	}
}
