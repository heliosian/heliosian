package store

import (
	"fmt"
	"strings"
)

const OrderColumn = "Order"

const keyDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

func CheckKey(key string) error {
	for _, r := range key {
		if !strings.ContainsRune(keyDigits, r) {
			return fmt.Errorf("order %q holds %q; an order is lowercase letters and digits", key, r)
		}
	}
	if strings.HasSuffix(key, "0") {
		return fmt.Errorf("order %q ends in 0", key)
	}
	return nil
}

func CompareKeys(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	return strings.Compare(a, b)
}

func Order(current []string) []string {
	length := make([]int, len(current))
	prev := make([]int, len(current))
	end := -1
	for i, key := range current {
		prev[i] = -1
		if key == "" {
			continue
		}
		length[i] = 1
		for j := range i {
			if current[j] != "" && current[j] < key && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}
		if end < 0 || length[i] > length[end] {
			end = i
		}
	}
	out := make([]string, len(current))
	for i := end; i >= 0; i = prev[i] {
		out[i] = current[i]
	}
	lo, from := "", 0
	for i := 0; i <= len(out); i++ {
		if i < len(out) && out[i] == "" {
			continue
		}
		hi := ""
		if i < len(out) {
			hi = out[i]
		}
		spread(out[from:i], lo, hi)
		lo, from = hi, i+1
	}
	return out
}

func spread(keys []string, lo, hi string) {
	if len(keys) == 0 {
		return
	}
	mid := len(keys) / 2
	keys[mid] = between(lo, hi)
	spread(keys[:mid], lo, keys[mid])
	spread(keys[mid+1:], keys[mid], hi)
}

// between is a key sorting after lo and before hi, either blank for no bound;
// no key ends in 0, so a longer key can always fit between two neighbours.
func between(lo, hi string) string {
	if hi != "" {
		n := 0
		for n < len(hi) && digitAt(lo, n) == hi[n] {
			n++
		}
		if n > 0 {
			return hi[:n] + between(tail(lo, n), hi[n:])
		}
	}
	a, b := 0, len(keyDigits)
	if lo != "" {
		a = strings.IndexByte(keyDigits, lo[0])
	}
	if hi != "" {
		b = strings.IndexByte(keyDigits, hi[0])
	}
	if b-a > 1 {
		return string(keyDigits[(a+b)/2])
	}
	if len(hi) > 1 {
		return hi[:1]
	}
	return string(keyDigits[a]) + between(tail(lo, 1), "")
}

func digitAt(key string, i int) byte {
	if i < len(key) {
		return key[i]
	}
	return '0'
}

func tail(key string, i int) string {
	if i < len(key) {
		return key[i:]
	}
	return ""
}
