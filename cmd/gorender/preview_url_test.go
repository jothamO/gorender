package main

import "testing"

func TestNormalizePreviewURL(t *testing.T) {
	cases := map[string]string{
		`http://127.0.0.1:8080/moments`:                   `http://127.0.0.1:8080/moments`,
		`"http://127.0.0.1:8080/moments"`:                 `http://127.0.0.1:8080/moments`,
		`\"http://127.0.0.1:8080/moments\"`:               `http://127.0.0.1:8080/moments`,
		`%22http://127.0.0.1:8080/moments%22`:             `http://127.0.0.1:8080/moments`,
		`%2522http://127.0.0.1:8080/moments%2522`:         `http://127.0.0.1:8080/moments`,
		`   "http://127.0.0.1:8080/moments-mm57lkdh"   `:  `http://127.0.0.1:8080/moments-mm57lkdh`,
	}
	for in, want := range cases {
		if got := normalizePreviewURL(in); got != want {
			t.Fatalf("normalizePreviewURL(%q) = %q, want %q", in, got, want)
		}
	}
}
