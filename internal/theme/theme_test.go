package theme

import (
	"strings"
	"testing"
)

func TestThemeReadsItsRows(t *testing.T) {
	row := func(key, value string) map[string]string { return map[string]string{"Key": key, "Value": value} }
	got, err := FromRows([]map[string]string{row("Intro", "hello"), row(SidebarKey, " #1A2B3C "), row(PageKey, "#ffffff"), row(PageEndKey, "#eeeeee")})
	if err != nil || got != (Theme{Sidebar: "#1a2b3c", Page: "#ffffff", PageEnd: "#eeeeee"}) {
		t.Errorf("theme = %+v, %v", got, err)
	}
	if got, err := FromRows([]map[string]string{row(LogoKey, "logos/"+strings.Repeat("a", 64)+".png")}); err != nil || got.Logo == "" {
		t.Errorf("logo: %+v, %v", got, err)
	}
	for _, bad := range [][]map[string]string{
		{row(LogoKey, "brand/logo.png")},
		{row(PageKey, "#fff")},
		{row(PageKey, "#ffffff"), row(PageKey, "#eeeeee")},
		{row(SidebarEndKey, "#eeeeee")},
	} {
		if _, err := FromRows(bad); err == nil {
			t.Errorf("rows %v read", bad)
		}
	}
	if v := (Theme{Sidebar: "#000000"}).Values(); len(v) != 7 || v[SidebarKey] != "#000000" || v[PageKey] != "" {
		t.Errorf("values = %v", v)
	}
}
