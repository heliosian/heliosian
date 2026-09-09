package main

import "testing"

func TestFlattenBioSeparatesParagraphsAndDecodesEntities(t *testing.T) {
	bio, err := flattenBio("<p>She holds a Bachelor&#39;s degree.</p>\n<p>Outside work, she\n  surfs.</p>")
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	want := "She holds a Bachelor's degree.\n\nOutside work, she surfs."
	if bio != want {
		t.Errorf("bio = %q, want %q", bio, want)
	}
}

// A link keeps its words and loses its address: the About Me card renders text.
func TestFlattenBioKeepsLinkTextAndDropsMarkup(t *testing.T) {
	bio, err := flattenBio(`<p>He works at <a href="https://example.org/">the lab</a>.<br>Ask him about it.</p>`)
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	want := "He works at the lab.\nAsk him about it."
	if bio != want {
		t.Errorf("bio = %q, want %q", bio, want)
	}
}

// The bio and the override say the same thing in the punctuation each was typed
// with, so the override is dead weight the load would refuse to carry.
func TestCaughtUpIgnoresPunctuationAndSpacing(t *testing.T) {
	if !caughtUp("Runs the front office.  Knows where everything is.",
		"Runs the front office. Knows where everything is.") {
		t.Error("an override differing only in spacing should count as caught up")
	}
	if !caughtUp("She's taught here for years", "She’s taught here for years.\n\nShe also sails.") {
		t.Error("an override the bio has grown past should count as caught up")
	}
}

// An override saying something of its own is a person's own words, and survives.
func TestCaughtUpLeavesADifferentOverrideAlone(t *testing.T) {
	if caughtUp("Ask me about the worm farm.", "Ruth has taught kindergarten for twelve years.") {
		t.Error("an override with its own content should be left alone")
	}
}

func TestFlattenBioOfNothingIsEmpty(t *testing.T) {
	bio, err := flattenBio("")
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if bio != "" {
		t.Errorf("bio = %q, want empty", bio)
	}
}
