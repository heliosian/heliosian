package db

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"heliosian/internal/store"
)

const (
	pdfRangePages = 20
	pdfRangeBytes = 16 << 20
)

type pageRange struct {
	first, last int
	body        []byte
}

type pdfPages struct {
	ctx   *model.Context
	total int
}

func readPDFPages(c context.Context, raw []byte) (pdfPages, error) {
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.EXTRACTPAGES
	ctx, err := api.ReadValidateAndOptimize(c, bytes.NewReader(raw), conf, nil)
	if err != nil {
		return pdfPages{}, fmt.Errorf("read the pdf's pages: %w", err)
	}
	return pdfPages{ctx: ctx, total: ctx.PageCount}, nil
}

func (p pdfPages) write(c context.Context, first, last int) ([]byte, error) {
	pages := []int{}
	for n := first; n <= last; n++ {
		pages = append(pages, n)
	}
	out, err := pdfcpu.ExtractPages(c, p.ctx, pages, false)
	if err != nil {
		return nil, fmt.Errorf("pages %d-%d: %w", first, last, err)
	}
	buf := &bytes.Buffer{}
	if err := api.WriteContext(c, out, buf); err != nil {
		return nil, fmt.Errorf("pages %d-%d: %w", first, last, err)
	}
	return buf.Bytes(), nil
}

func (p pdfPages) ranges(c context.Context, most int) ([]pageRange, error) {
	out := []pageRange{}
	for first := 1; first <= p.total; {
		n := min(most, p.total-first+1)
		for {
			body, err := p.write(c, first, first+n-1)
			if err != nil {
				return nil, err
			}
			if len(body) <= pdfRangeBytes || n == 1 {
				out = append(out, pageRange{first: first, last: first + n - 1, body: body})
				break
			}
			n = max(1, n*pdfRangeBytes/len(body))
		}
		first += n
	}
	return out, nil
}

func pageSpan(name string) (first, total int) {
	var last int
	if _, err := fmt.Sscanf(name, "pages %d–%d of %d", &first, &last, &total); err == nil {
		return first, total
	}
	if _, err := fmt.Sscanf(name, "page %d of %d", &first, &total); err == nil {
		return first, total
	}
	return 1, 0
}

func spanName(first, last, total int) string {
	if first == last {
		return fmt.Sprintf("page %d of %d", first, total)
	}
	return fmt.Sprintf("pages %d–%d of %d", first, last, total)
}

func pageChildren(doc store.Row, p pdfPages, ranges []pageRange) []extracted {
	offset, total := pageSpan(doc["name"])
	if doc["relation"] != "pages" || total == 0 {
		offset, total = 1, p.total
	}
	out := []extracted{}
	for _, r := range ranges {
		out = append(out, extracted{relation: "pages", body: r.body, mime: pdfType, title: spanName(offset+r.first-1, offset+r.last-1, total)})
	}
	return out
}
