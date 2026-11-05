package controllers

import "strconv"

// Page describes one page of a long list (catalog, admin orders, users).
type Page struct {
	Number     int   // 1-based
	Size       int   // items per page
	Total      int64 // items in the whole list
	TotalPages int
}

// newPage reads ?page= and ?limit= safely. Anything strange falls back to a default:
// before, ?limit=0 crashed the template (division by zero) and ?limit=1000000
// loaded the whole catalog in one request.
func newPage(pageParam, limitParam string, defaultSize, maxSize int) Page {
	number, err := strconv.Atoi(pageParam)
	if err != nil || number < 1 {
		number = 1
	}
	size, err := strconv.Atoi(limitParam)
	if err != nil || size < 1 || size > maxSize {
		size = defaultSize
	}
	return Page{Number: number, Size: size}
}

// SetTotal stores the item count and works out how many pages there are.
func (p *Page) SetTotal(total int64) {
	p.Total = total
	// Integer division that rounds UP: 25 items by 12 = 3 pages, not 2.
	// (The old template did float64(25/12) = 2.0 — the remainder was already lost.)
	p.TotalPages = int((total + int64(p.Size) - 1) / int64(p.Size))
	if p.TotalPages < 1 {
		p.TotalPages = 1 // an empty list is still "page 1 of 1"
	}
}

func (p Page) Offset() int   { return (p.Number - 1) * p.Size }
func (p Page) HasPrev() bool { return p.Number > 1 }
func (p Page) HasNext() bool { return p.Number < p.TotalPages }
func (p Page) Prev() int     { return p.Number - 1 }
func (p Page) Next() int     { return p.Number + 1 }
