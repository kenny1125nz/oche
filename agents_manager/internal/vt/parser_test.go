package vt

import (
	"strings"
	"testing"
)

func TestBasicText(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("hello"))
	cx, cy := p.Active.Cursor()
	if cx != 5 || cy != 0 {
		t.Fatalf("expected cursor at (5,0), got (%d,%d)", cx, cy)
	}
	ch := p.Active.cells[0][0]
	if ch.Char != 'h' {
		t.Fatalf("expected 'h' at (0,0), got %q", ch.Char)
	}
}

func TestCursorMovement(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("hello"))
	p.Feed([]byte("\r\n"))
	cx, cy := p.Active.Cursor()
	if cx != 0 || cy != 1 {
		t.Fatalf("expected cursor at (0,1) after CRLF, got (%d,%d)", cx, cy)
	}

	p2 := NewParser(80, 24)
	p2.Feed([]byte("\033[5;10H"))
	cx, cy = p2.Active.Cursor()
	if cx != 9 || cy != 4 {
		t.Fatalf("expected cursor at (9,4) after CUP 5;10, got (%d,%d)", cx, cy)
	}

	p2.Feed([]byte("\033[2A"))
	cx, cy = p2.Active.Cursor()
	if cx != 9 || cy != 2 {
		t.Fatalf("expected cursor at (9,2) after CUU 2, got (%d,%d)", cx, cy)
	}
}

func TestSGR(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[1;31;42mbold red on green"))
	_, _ = p.Active.Cursor()
	cell := p.Active.cells[0][0]
	if cell.Char != 'b' {
		t.Fatalf("expected 'b' at (0,0), got %q", cell.Char)
	}
	if cell.Attr&AttrBold == 0 {
		t.Fatal("expected bold attribute")
	}
	if cell.Fg.Type != ColorANSI || cell.Fg.Index != 1 {
		t.Fatalf("expected red fg (ANSI 1), got type=%d idx=%d", cell.Fg.Type, cell.Fg.Index)
	}
	if cell.Bg.Type != ColorANSI || cell.Bg.Index != 2 {
		t.Fatalf("expected green bg (ANSI 2), got type=%d idx=%d", cell.Bg.Type, cell.Bg.Index)
	}

	p2 := NewParser(80, 24)
	p2.Feed([]byte("\033[38;5;82m256color"))
	cell2 := p2.Active.cells[0][0]
	if cell2.Fg.Type != ColorANSI256 || cell2.Fg.Index != 82 {
		t.Fatalf("expected 256color fg 82, got type=%d idx=%d", cell2.Fg.Type, cell2.Fg.Index)
	}

	p3 := NewParser(80, 24)
	p3.Feed([]byte("\033[38;2;255;128;0mtruecolor"))
	cell3 := p3.Active.cells[0][0]
	if cell3.Fg.Type != ColorRGB || cell3.Fg.R != 255 || cell3.Fg.G != 128 || cell3.Fg.B != 0 {
		t.Fatalf("expected RGB fg 255,128,0, got type=%d", cell3.Fg.Type)
	}
}

func TestSGRReset(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[1;31mbold red"))
	p.Feed([]byte("\033[0mnormal"))
	cell := p.Active.cells[0][10]
	if cell.Attr != 0 {
		t.Fatal("expected attributes reset after SGR 0")
	}
	if !cell.Fg.IsDefault() {
		t.Fatal("expected default fg after SGR 0")
	}
}

func TestEraseDisplay(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("line1\r\nline2\r\nline3"))
	cx, cy := p.Active.Cursor()
	if cx != 5 || cy != 2 {
		t.Fatalf("expected cursor at (5,2) after three lines, got (%d,%d)", cx, cy)
	}
	p.Feed([]byte("\033[2J"))
	// cursor unchanged after ED
	cx, cy = p.Active.Cursor()
	if cx != 5 || cy != 2 {
		t.Fatalf("expected cursor unchanged at (5,2) after ED 2, got (%d,%d)", cx, cy)
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 80; x++ {
			if !p.Active.cells[y][x].IsEmpty() {
				t.Fatalf("expected cell (%d,%d) to be empty after ED 2", x, y)
			}
		}
	}
}

func TestEraseLine(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("hello world"))
	p.Feed([]byte("\033[K"))
	for x := 11; x < 80; x++ {
		if !p.Active.cells[0][x].IsEmpty() {
			t.Fatalf("expected cell (%d,0) to be empty after EL", x)
		}
	}
	if p.Active.cells[0][0].Char != 'h' {
		t.Fatal("expected 'h' to remain after EL 0")
	}
}

func TestScroll(t *testing.T) {
	p := NewParser(80, 5)
	// write 5 lines with CRLF, then one more line without trailing \n
	for i := 0; i < 5; i++ {
		p.Feed([]byte("line\r\n"))
	}
	p.Feed([]byte("line"))
	cx, cy := p.Active.Cursor()
	if cx != 4 || cy != 4 {
		t.Fatalf("expected cursor at (4,4), got (%d,%d)", cx, cy)
	}
	// first line should be at row 0 (it's the first "line" written)
	if p.Active.cells[0][0].Char != 'l' {
		t.Fatalf("expected first 'line' at (0,0), got %q", p.Active.cells[0][0].Char)
	}
	if p.Active.cells[4][0].Char != 'l' {
		t.Fatalf("expected fifth 'line' at (4,0), got %q at (0,4)", p.Active.cells[4][0].Char)
	}
}

func TestAlternateScreen(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("normal screen content"))
	p.Feed([]byte("\033[?1049h"))
	if p.Active != p.Alt {
		t.Fatal("expected active to be alt screen after ?1049h")
	}
	if !p.Active.cells[0][0].IsEmpty() {
		t.Fatal("expected alt screen to start empty")
	}
	p.Feed([]byte("alt screen content"))
	if p.Active.cells[0][0].Char != 'a' {
		t.Fatalf("expected 'a' on alt screen, got %q", p.Active.cells[0][0].Char)
	}
	p.Feed([]byte("\033[?1049l"))
	if p.Active != p.Normal {
		t.Fatal("expected active to be normal screen after ?1049l")
	}
	if p.Active.cells[0][0].Char != 'n' {
		t.Fatalf("expected 'n' preserved on normal screen, got %q", p.Active.cells[0][0].Char)
	}
	if p.Alt != nil {
		t.Fatal("expected alt screen to be nil after exit")
	}
}

func TestAutoWrap(t *testing.T) {
	p := NewParser(10, 3)
	p.Feed([]byte("1234567890ab"))
	cx, cy := p.Active.Cursor()
	if cx != 2 || cy != 1 {
		t.Fatalf("expected cursor at (2,1) after wrap, got (%d,%d)", cx, cy)
	}
	cell := p.Active.cells[1][0]
	if cell.Char != 'a' {
		t.Fatalf("expected 'a' at (0,1) after wrap, got %q", cell.Char)
	}
}

func TestModeTracking(t *testing.T) {
	p := NewParser(80, 24)
	if p.Active.cursorVisible != true {
		t.Fatal("expected cursor visible by default")
	}
	p.Feed([]byte("\033[?25l"))
	if p.Active.cursorVisible != false {
		t.Fatal("expected cursor hidden after ?25l")
	}
	p.Feed([]byte("\033[?25h"))
	if p.Active.cursorVisible != true {
		t.Fatal("expected cursor visible after ?25h")
	}

	if p.Active.decCKM != false {
		t.Fatal("expected DECCKM off by default")
	}
	p.Feed([]byte("\033[?1h"))
	if p.Active.decCKM != true {
		t.Fatal("expected DECCKM on after ?1h")
	}
	p.Feed([]byte("\033[?1l"))
	if p.Active.decCKM != false {
		t.Fatal("expected DECCKM off after ?1l")
	}

	if p.Active.bracketedPaste != false {
		t.Fatal("expected bracketed paste off by default")
	}
	p.Feed([]byte("\033[?2004h"))
	if p.Active.bracketedPaste != true {
		t.Fatal("expected bracketed paste on after ?2004h")
	}
}

func TestFullScreenSerialization(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("Hello, World!"))
	p.Feed([]byte("\033[5;10H"))
	p.Feed([]byte("\033[1;31mRed Bold"))
	result := p.ScreenSnapshot()
	if !strings.Contains(result, "Hello, World!") {
		t.Fatal("expected serialization to contain 'Hello, World!'")
	}
	if !strings.Contains(result, "\033[1;38;5;1m") && !strings.Contains(result, "\033[1;31m") {
		t.Fatal("expected serialization to contain SGR for bold red")
	}
	if !strings.Contains(result, "Red Bold") {
		t.Fatal("expected serialization to contain 'Red Bold'")
	}
	if !strings.Contains(result, "\033[5;18H") {
		t.Fatalf("expected serialization to position cursor at 5,18 (after 'Red Bold'), got %q", result)
	}
}

func TestSerializationRoundTrip(t *testing.T) {
	p := NewParser(80, 24)
	input := "\033[1;32mGreen Bold\033[0m\n\033[38;5;82m256color\033[0m\n\033[38;2;255;0;0mRed"
	p.Feed([]byte(input))
	snapshot := p.ScreenSnapshot()
	p2 := NewParser(80, 24)
	p2.Feed([]byte(snapshot))
	normalize := func(c Cell) Cell {
		if c.Char == 0 {
			c.Char = ' '
		}
		// ANSI 0-15 and ANSI256 0-15 are equivalent
		if c.Fg.Type == ColorANSI256 && c.Fg.Index < 16 {
			c.Fg.Type = ColorANSI
		}
		if c.Bg.Type == ColorANSI256 && c.Bg.Index < 16 {
			c.Bg.Type = ColorANSI
		}
		return c
	}
	for y := 0; y < 3 && y < 24; y++ {
		for x := 0; x < 80; x++ {
			c1 := normalize(p.Active.cells[y][x])
			c2 := normalize(p2.Active.cells[y][x])
			if c1.Char != c2.Char || c1.Attr != c2.Attr || c1.Fg != c2.Fg || c1.Bg != c2.Bg {
				t.Fatalf("cell mismatch at (%d,%d): orig=%+v roundtrip=%+v", x, y, c1, c2)
			}
		}
	}
}

func TestScrollRegion(t *testing.T) {
	p := NewParser(80, 5)
	p.Feed([]byte("X"))         // mark row 0 col 0
	p.Feed([]byte("\033[2;4r")) // scroll region rows 1-3 (0-indexed: top=1, bottom=3)
	p.Feed([]byte("\033[2;1H")) // cursor to top of scroll region (row 1, col 0)
	// Feed 3 lines to fill the 3-line region
	p.Feed([]byte("A\r\nB\r\nC"))
	// Row 0 (outside region) unchanged
	if p.Active.cells[0][0].Char != 'X' {
		t.Fatalf("expected 'X' preserved at (0,0), got %q", p.Active.cells[0][0].Char)
	}
	// Scroll region rows filled
	if p.Active.cells[1][0].Char != 'A' {
		t.Fatalf("expected 'A' at (0,1), got %q", p.Active.cells[1][0].Char)
	}
	if p.Active.cells[3][0].Char != 'C' {
		t.Fatalf("expected 'C' at (0,3), got %q", p.Active.cells[3][0].Char)
	}
	// One more line: scroll region scrolls, A is lost
	p.Feed([]byte("\r\nD"))
	if p.Active.cells[1][0].Char != 'B' {
		t.Fatalf("expected 'B' at (0,1) after scroll, got %q", p.Active.cells[1][0].Char)
	}
	if p.Active.cells[3][0].Char != 'D' {
		t.Fatalf("expected 'D' at (0,3) after scroll, got %q", p.Active.cells[3][0].Char)
	}
	// Row 0 still unchanged
	if p.Active.cells[0][0].Char != 'X' {
		t.Fatal("expected row 0 unchanged after scroll region scroll")
	}
}

func TestInsertDeleteLines(t *testing.T) {
	p := NewParser(80, 5)
	p.Feed([]byte("AAA\r\nBBB\r\nCCC\r\nDDD\r\nEEE"))
	p.Feed([]byte("\033[2;1H")) // cursor to (0,1)
	p.Feed([]byte("\033[2M"))   // delete 2 lines from row 1
	// Row 0 unchanged
	if p.Active.cells[0][0].Char != 'A' {
		t.Fatalf("expected 'A' preserved at (0,0), got %q", p.Active.cells[0][0].Char)
	}
	// Row 1 now has DDD (shifted up from row 3)
	if p.Active.cells[1][0].Char != 'D' {
		t.Fatalf("expected 'D' at (0,1) after delete, got %q", p.Active.cells[1][0].Char)
	}
	// Row 2 now has EEE (shifted up from row 4)
	if p.Active.cells[2][0].Char != 'E' {
		t.Fatalf("expected 'E' at (0,2) after delete, got %q", p.Active.cells[2][0].Char)
	}
	// Rows 3-4 cleared
	if !p.Active.cells[3][0].IsEmpty() {
		t.Fatal("expected row 3 cleared after delete")
	}
}

func TestColor256(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[38;5;196m"))
	p.Feed([]byte("red256"))
	cell := p.Active.cells[0][0]
	if cell.Fg.Type != ColorANSI256 || cell.Fg.Index != 196 {
		t.Fatalf("expected 256color 196, got type=%d idx=%d", cell.Fg.Type, cell.Fg.Index)
	}
}

func TestTrueColor(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[48;2;100;200;50m"))
	p.Feed([]byte("bg"))
	cell := p.Active.cells[0][0]
	if cell.Bg.Type != ColorRGB || cell.Bg.R != 100 || cell.Bg.G != 200 || cell.Bg.B != 50 {
		t.Fatalf("expected RGB bg 100,200,50, got type=%d %d,%d,%d", cell.Bg.Type, cell.Bg.R, cell.Bg.G, cell.Bg.B)
	}
}

func TestReverseVideo(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[7mreversed"))
	cell := p.Active.cells[0][0]
	if cell.Attr&AttrReverse == 0 {
		t.Fatal("expected reversed attribute")
	}
	p.Feed([]byte("\033[27m"))
	p.Feed([]byte("normal"))
	cell = p.Active.cells[0][8]
	if cell.Attr&AttrReverse != 0 {
		t.Fatal("expected reverse cleared")
	}
}

func TestCRLF(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("abc\r\ndef"))
	cx, cy := p.Active.Cursor()
	if cx != 3 || cy != 1 {
		t.Fatalf("expected cursor at (3,1) after CRLF, got (%d,%d)", cx, cy)
	}
}

func TestTab(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("a\tb"))
	if p.Active.cells[0][0].Char != 'a' {
		t.Fatal("expected 'a' at (0,0)")
	}
	if p.Active.cells[0][8].Char != 'b' {
		t.Fatalf("expected 'b' at (8,0) after tab, got %q at col 8", p.Active.cells[0][8].Char)
	}
}

func TestBackspace(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("ab\bc"))
	if p.Active.cells[0][0].Char != 'a' {
		t.Fatal("expected 'a' at (0,0)")
	}
	if p.Active.cells[0][1].Char != 'c' {
		t.Fatalf("expected 'c' at (1,0) after backspace, got %q", p.Active.cells[0][1].Char)
	}
}

func TestEmptyInput(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed(nil)
	p.Feed([]byte{})
	cx, cy := p.Active.Cursor()
	if cx != 0 || cy != 0 {
		t.Fatalf("expected cursor at origin after empty input, got (%d,%d)", cx, cy)
	}
}

func TestInsertMode(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("abfgh"))
	p.Feed([]byte("\033[1;3H"))    // cursor to (2,0) — before 'f'
	p.Feed([]byte("\033[4h"))      // insert mode on
	p.Feed([]byte("cde"))          // insert "cde" before "fgh"
	if p.Active.cells[0][0].Char != 'a' {
		t.Fatalf("expected 'a' at (0,0), got %q", p.Active.cells[0][0].Char)
	}
	if p.Active.cells[0][2].Char != 'c' {
		t.Fatalf("expected 'c' at (2,0), got %q", p.Active.cells[0][2].Char)
	}
	if p.Active.cells[0][5].Char != 'f' {
		t.Fatalf("expected 'f' shifted to (5,0), got %q", p.Active.cells[0][5].Char)
	}
}

func TestDeleteChars(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("abcdef"))
	p.Feed([]byte("\033[1;3H")) // cursor to (2,0)
	p.Feed([]byte("\033[2P"))   // delete 2 chars at cursor
	if p.Active.cells[0][0].Char != 'a' {
		t.Fatalf("expected 'a' at (0,0), got %q", p.Active.cells[0][0].Char)
	}
	if p.Active.cells[0][2].Char != 'e' {
		t.Fatalf("expected 'e' at (2,0) after DCH 2 (c,d removed), got %q", p.Active.cells[0][2].Char)
	}
}

func TestInsertChars(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("abcdef"))
	p.Feed([]byte("\033[1;3H")) // cursor to (2,0)
	p.Feed([]byte("\033[2@"))   // insert 2 chars (shift right)
	if p.Active.cells[0][2].Char != 0 {
		t.Fatalf("expected empty at (2,0) after ICH 2, got %q", p.Active.cells[0][2].Char)
	}
	if p.Active.cells[0][4].Char != 'c' {
		t.Fatalf("expected 'c' shifted to (4,0) after ICH 2, got %q", p.Active.cells[0][4].Char)
	}
}

func TestCursorNextPrevLine(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("hello"))
	p.Feed([]byte("\033[E")) // cursor next line, column 0
	cx, cy := p.Active.Cursor()
	if cx != 0 || cy != 1 {
		t.Fatalf("expected cursor at (0,1) after CNL, got (%d,%d)", cx, cy)
	}
	p.Feed([]byte("\033[F")) // cursor previous line, column 0
	cx, cy = p.Active.Cursor()
	if cx != 0 || cy != 0 {
		t.Fatalf("expected cursor at (0,0) after CPL, got (%d,%d)", cx, cy)
	}
}

func TestSaveRestoreCursor(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[5;10H"))
	p.Feed([]byte("\033[s")) // save cursor
	p.Feed([]byte("\033[1;1H"))
	p.Feed([]byte("\033[u")) // restore cursor
	cx, cy := p.Active.Cursor()
	if cx != 9 || cy != 4 {
		t.Fatalf("expected cursor restored to (9,4), got (%d,%d)", cx, cy)
	}
}

func TestDecSaveRestore(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[5;10H"))
	p.Feed([]byte("\0337")) // DECSC
	p.Feed([]byte("\033[1;1H"))
	p.Feed([]byte("\0338")) // DECRC
	cx, cy := p.Active.Cursor()
	if cx != 9 || cy != 4 {
		t.Fatalf("expected cursor restored to (9,4) via DECSC/DECRC, got (%d,%d)", cx, cy)
	}
}

func TestAutoWrapOff(t *testing.T) {
	p := NewParser(10, 3)
	p.Feed([]byte("\033[?7l")) // disable auto-wrap
	p.Feed([]byte("1234567890ab"))
	cx, cy := p.Active.Cursor()
	if cx != 12 || cy != 0 {
		t.Fatalf("expected cursor at (12,0) with auto-wrap off (chars past margin lost), got (%d,%d)", cx, cy)
	}
	// Only first 10 chars fit; the rest are clipped by WriteCell bounds check
	if p.Active.cells[0][9].Char != '0' {
		t.Fatalf("expected '0' at (9,0), got %q", p.Active.cells[0][9].Char)
	}
}

func TestDeltaSerialization(t *testing.T) {
	p1 := NewParser(80, 24)
	p1.Feed([]byte("original content"))
	p2 := NewParser(80, 24)
	p2.Feed([]byte("original content"))
	p2.Feed([]byte("\033[5;10H"))
	p2.Feed([]byte("modified"))
	delta := p2.Active.Serialize(p1.Active)
	if !strings.Contains(delta, "modified") {
		t.Fatal("expected delta to contain 'modified'")
	}
	if strings.Contains(delta, "original content") {
		t.Fatal("expected delta to not include unchanged rows")
	}
}

func TestRIS(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[1;31mHello"))
	p.Feed([]byte("\033c"))
	cx, cy := p.Active.Cursor()
	if cx != 0 || cy != 0 {
		t.Fatalf("expected cursor at origin after RIS, got (%d,%d)", cx, cy)
	}
	if !p.Active.cells[0][0].IsEmpty() {
		t.Fatal("expected screen cleared after RIS")
	}
	p.Feed([]byte("test"))
	cell := p.Active.cells[0][0]
	if cell.Attr != 0 || cell.Char != 't' {
		t.Fatal("expected attributes reset after RIS")
	}
}

func TestOriginMode(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("\033[10;20r")) // scroll region 10-20 (1-indexed)
	p.Feed([]byte("\033[?6h"))    // origin mode on
	p.Feed([]byte("\033[2;2H"))   // CUP to row 2, col 2
	_, cy := p.Active.Cursor()
	// In origin mode, CUP row is relative to scroll region top.
	// Row 2 (1-indexed) means row (10 + 2 - 1) = 11 (1-indexed) = 10 (0-indexed)
	if cy != 10 {
		t.Fatalf("expected cursor at row 10 (0-indexed) in origin mode, got %d", cy)
	}
}

func TestIndexAndReverseIndex(t *testing.T) {
	p := NewParser(80, 5)
	p.Feed([]byte("A\r\nB\r\nC\r\nD\r\nE"))
	p.Feed([]byte("\033D")) // Index
	_, cy := p.Active.Cursor()
	if cy != 4 {
		t.Fatalf("expected cursor at row 4 after index, got %d", cy)
	}
	p2 := NewParser(80, 5)
	p2.Feed([]byte("\033[2;4r"))
	p2.Feed([]byte("\033[2;1H"))
	p2.Feed([]byte("\033M")) // Reverse Index
	_, cy = p2.Active.Cursor()
	if cy != 1 {
		t.Fatalf("expected cursor at row 1 after RI, got %d", cy)
	}
}

func TestNextLine(t *testing.T) {
	p := NewParser(80, 24)
	p.Feed([]byte("hello"))
	p.Feed([]byte("\033E")) // NEL - next line
	cx, cy := p.Active.Cursor()
	if cx != 0 || cy != 1 {
		t.Fatalf("expected cursor at (0,1) after NEL, got (%d,%d)", cx, cy)
	}
}
