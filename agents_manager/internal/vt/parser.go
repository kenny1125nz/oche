package vt

import (
	"fmt"
	"strings"
)

type ColorType int

const (
	ColorDefault ColorType = iota
	ColorANSI
	ColorANSI256
	ColorRGB
)

type Color struct {
	Type  ColorType
	Index uint8
	R, G, B uint8
}

var (
	ColorDefaultFG = Color{Type: ColorDefault}
	ColorDefaultBG = Color{Type: ColorDefault}
)

func (c Color) IsDefault() bool { return c.Type == ColorDefault }

type Attr int

const (
	AttrNone      Attr = 0
	AttrBold      Attr = 1 << 0
	AttrDim       Attr = 1 << 1
	AttrItalic    Attr = 1 << 2
	AttrUnderline Attr = 1 << 3
	AttrBlink     Attr = 1 << 4
	AttrReverse   Attr = 1 << 5
	AttrStrike    Attr = 1 << 6
)

type Cell struct {
	Char rune
	Fg   Color
	Bg   Color
	Attr Attr
}

func (c Cell) IsEmpty() bool {
	return c.Char == 0 || c.Char == ' '
}

type Screen struct {
	Width  int
	Height int
	cells  [][]Cell
	cx, cy int

	topMargin   int
	bottomMargin int

	autoWrap      bool
	cursorVisible bool
	decCKM        bool
	bracketedPaste bool
	originMode    bool

	insertMode bool
}

func NewScreen(w, h int) *Screen {
	s := &Screen{
		Width:         w,
		Height:        h,
		autoWrap:      true,
		cursorVisible: true,
		topMargin:     0,
		bottomMargin:  h - 1,
	}
	s.cells = make([][]Cell, h)
	for y := 0; y < h; y++ {
		s.cells[y] = make([]Cell, w)
	}
	return s
}

func (s *Screen) Resize(w, h int) {
	if w == s.Width && h == s.Height {
		return
	}
	cells := make([][]Cell, h)
	for y := 0; y < h; y++ {
		cells[y] = make([]Cell, w)
	}
	copyH := h
	if s.Height < copyH {
		copyH = s.Height
	}
	copyW := w
	if s.Width < copyW {
		copyW = s.Width
	}
	for y := 0; y < copyH; y++ {
		for x := 0; x < copyW; x++ {
			if y < s.Height && x < s.Width {
				cells[y][x] = s.cells[y][x]
			}
		}
	}
	s.cells = cells
	s.Width = w
	s.Height = h
	s.bottomMargin = h - 1
	if s.cy >= h {
		s.cy = h - 1
	}
	if s.cx >= w {
		s.cx = w - 1
	}
	if s.topMargin > s.bottomMargin {
		s.topMargin = 0
	}
}

func (s *Screen) SetCell(x, y int, ch rune) {
	if x < 0 || x >= s.Width || y < 0 || y >= s.Height {
		return
	}
	cell := &s.cells[y][x]
	cell.Char = ch
	if ch != 0 && ch != ' ' {
		cell.Fg = ColorDefaultFG
		cell.Bg = ColorDefaultBG
	}
}

func (s *Screen) WriteCell(x, y int, ch rune, fg, bg Color, attr Attr) {
	if x < 0 || x >= s.Width || y < 0 || y >= s.Height {
		return
	}
	cell := &s.cells[y][x]
	cell.Char = ch
	cell.Fg = fg
	cell.Bg = bg
	cell.Attr = attr
}

func (s *Screen) Cursor() (x, y int) {
	return s.cx, s.cy
}

func (s *Screen) SetCursor(x, y int) {
	if s.originMode {
		y += s.topMargin
	}
	if x < 0 {
		x = 0
	}
	if x >= s.Width {
		x = s.Width - 1
	}
	if y < s.topMargin {
		y = s.topMargin
	}
	if y > s.bottomMargin {
		y = s.bottomMargin
	}
	s.cx = x
	s.cy = y
}

func (s *Screen) CursorUp(n int) {
	s.SetCursor(s.cx, s.cy-n)
}

func (s *Screen) CursorDown(n int) {
	s.SetCursor(s.cx, s.cy+n)
}

func (s *Screen) CursorForward(n int) {
	s.SetCursor(s.cx+n, s.cy)
}

func (s *Screen) CursorBackward(n int) {
	s.SetCursor(s.cx-n, s.cy)
}

func (s *Screen) LineFeed() {
	s.cy++
	if s.cy > s.bottomMargin {
		s.ScrollUp(1)
		s.cy = s.bottomMargin
	}
}

func (s *Screen) ReverseLineFeed() {
	s.cy--
	if s.cy < s.topMargin {
		s.ScrollDown(1)
		s.cy = s.topMargin
	}
}

func (s *Screen) CarriageReturn() {
	s.cx = 0
}

func (s *Screen) Tab() {
	next := (s.cx + 8) & ^7
	if next >= s.Width {
		next = s.Width - 1
	}
	s.cx = next
}

func (s *Screen) Backspace() {
	if s.cx > 0 {
		s.cx--
	}
}

func (s *Screen) PutRune(ch rune, fg, bg Color, attr Attr) {
	if s.insertMode && s.cx < s.Width-1 {
		copy(s.cells[s.cy][s.cx+1:], s.cells[s.cy][s.cx:s.Width-1])
		s.cells[s.cy][s.cx] = Cell{}
	}
	s.WriteCell(s.cx, s.cy, ch, fg, bg, attr)
	s.cx++
	if s.autoWrap && s.cx >= s.Width {
		s.cx = 0
		s.LineFeed()
	}
}

func (s *Screen) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	regionH := s.bottomMargin - s.topMargin + 1
	if n > regionH {
		n = regionH
	}
	for y := s.topMargin; y <= s.bottomMargin-n; y++ {
		copy(s.cells[y], s.cells[y+n])
	}
	for y := s.bottomMargin - n + 1; y <= s.bottomMargin; y++ {
		for x := 0; x < s.Width; x++ {
			s.cells[y][x] = Cell{}
		}
	}
}

func (s *Screen) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	regionH := s.bottomMargin - s.topMargin + 1
	if n > regionH {
		n = regionH
	}
	for y := s.bottomMargin; y >= s.topMargin+n; y-- {
		copy(s.cells[y], s.cells[y-n])
	}
	for y := s.topMargin; y < s.topMargin+n; y++ {
		for x := 0; x < s.Width; x++ {
			s.cells[y][x] = Cell{}
		}
	}
}

func (s *Screen) InsertLines(n int) {
	if s.cy < s.topMargin || s.cy > s.bottomMargin {
		return
	}
	regionH := s.bottomMargin - s.cy + 1
	if n > regionH {
		n = regionH
	}
	for y := s.bottomMargin; y >= s.cy+n; y-- {
		copy(s.cells[y], s.cells[y-n])
	}
	for y := s.cy; y < s.cy+n; y++ {
		for x := 0; x < s.Width; x++ {
			s.cells[y][x] = Cell{}
		}
	}
}

func (s *Screen) DeleteLines(n int) {
	if s.cy < s.topMargin || s.cy > s.bottomMargin {
		return
	}
	regionH := s.bottomMargin - s.cy + 1
	if n > regionH {
		n = regionH
	}
	for y := s.cy; y <= s.bottomMargin-n; y++ {
		copy(s.cells[y], s.cells[y+n])
	}
	for y := s.bottomMargin - n + 1; y <= s.bottomMargin; y++ {
		for x := 0; x < s.Width; x++ {
			s.cells[y][x] = Cell{}
		}
	}
}

func (s *Screen) InsertChars(n int) {
	if n <= 0 {
		return
	}
	if s.cx+n > s.Width {
		n = s.Width - s.cx
	}
	copy(s.cells[s.cy][s.cx+n:], s.cells[s.cy][s.cx:s.Width-n])
	for x := s.cx; x < s.cx+n && x < s.Width; x++ {
		s.cells[s.cy][x] = Cell{}
	}
}

func (s *Screen) DeleteChars(n int) {
	if n <= 0 {
		return
	}
	if s.cx+n > s.Width {
		n = s.Width - s.cx
	}
	copy(s.cells[s.cy][s.cx:], s.cells[s.cy][s.cx+n:])
	for x := s.Width - n; x < s.Width; x++ {
		s.cells[s.cy][x] = Cell{}
	}
}

func (s *Screen) EraseDisplay(mode int) {
	switch mode {
	case 0:
		s.EraseLine(0)
		for y := s.cy + 1; y < s.Height; y++ {
			for x := 0; x < s.Width; x++ {
				s.cells[y][x] = Cell{}
			}
		}
	case 1:
		for y := 0; y < s.cy; y++ {
			for x := 0; x < s.Width; x++ {
				s.cells[y][x] = Cell{}
			}
		}
		for x := 0; x <= s.cx; x++ {
			s.cells[s.cy][x] = Cell{}
		}
	case 2, 3:
		for y := 0; y < s.Height; y++ {
			for x := 0; x < s.Width; x++ {
				s.cells[y][x] = Cell{}
			}
		}
	}
}

func (s *Screen) EraseLine(mode int) {
	switch mode {
	case 0:
		for x := s.cx; x < s.Width; x++ {
			s.cells[s.cy][x] = Cell{}
		}
	case 1:
		for x := 0; x <= s.cx; x++ {
			s.cells[s.cy][x] = Cell{}
		}
	case 2:
		for x := 0; x < s.Width; x++ {
			s.cells[s.cy][x] = Cell{}
		}
	}
}

func (s *Screen) EraseChars(n int) {
	if n <= 0 {
		return
	}
	end := s.cx + n
	if end > s.Width {
		end = s.Width
	}
	for x := s.cx; x < end; x++ {
		s.cells[s.cy][x] = Cell{}
	}
}

func (s *Screen) SetScrollRegion(top, bottom int) {
	if top < 0 {
		top = 0
	}
	if bottom >= s.Height {
		bottom = s.Height - 1
	}
	if top < bottom {
		s.topMargin = top
		s.bottomMargin = bottom
	}
	if s.cy < s.topMargin {
		s.cy = s.topMargin
	}
	if s.cy > s.bottomMargin {
		s.cy = s.bottomMargin
	}
}

func (s *Screen) ResetScrollRegion() {
	s.topMargin = 0
	s.bottomMargin = s.Height - 1
}

func (s *Screen) Index() {
	s.cy++
	if s.cy > s.bottomMargin {
		s.ScrollUp(1)
		s.cy = s.bottomMargin
	}
}

func (s *Screen) ReverseIndex() {
	s.cy--
	if s.cy < s.topMargin {
		s.ScrollDown(1)
		s.cy = s.topMargin
	}
}

func (s *Screen) NextLine() {
	s.cx = 0
	s.LineFeed()
}

func (s *Screen) HorizontalPositionAbsolute(x int) {
	if x < 0 {
		x = 0
	}
	if x >= s.Width {
		x = s.Width - 1
	}
	s.cx = x
}

func (s *Screen) VerticalPositionAbsolute(y int) {
	s.SetCursor(s.cx, y)
}

func (s *Screen) HorizontalVerticalPosition(x, y int) {
	s.SetCursor(x, y)
}

func (s *Screen) CursorNextLine(n int) {
	s.cy += n
	if s.cy > s.bottomMargin {
		s.cy = s.bottomMargin
	}
	s.cx = 0
}

func (s *Screen) CursorPreviousLine(n int) {
	s.cy -= n
	if s.cy < s.topMargin {
		s.cy = s.topMargin
	}
	s.cx = 0
}

func (s *Screen) Serialize(prev *Screen) string {
	var b strings.Builder
	if prev == nil {
		s.serializeFull(&b)
	} else {
		s.serializeDelta(&b, prev)
	}
	return b.String()
}

func (s *Screen) serializeFull(b *strings.Builder) {
	b.WriteString("\033[?25l")
	b.WriteString("\033[2J")
	b.WriteString("\033[m")
	b.WriteString("\033[H")
	if !s.autoWrap {
		b.WriteString("\033[?7l")
	}
	for y := 0; y < s.Height; y++ {
		rowEmpty := true
		for x := 0; x < s.Width; x++ {
			if !s.cells[y][x].IsEmpty() {
				rowEmpty = false
				break
			}
		}
		if rowEmpty && y > 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\033[%d;%dH", y+1, 1))
		s.serializeRow(b, y)
	}
	if s.cursorVisible {
		b.WriteString("\033[?25h")
	}
	b.WriteString(fmt.Sprintf("\033[%d;%dH", s.cy+1, s.cx+1))
}

func (s *Screen) serializeDelta(b *strings.Builder, prev *Screen) {
	for y := 0; y < s.Height; y++ {
		changed := false
		if y >= prev.Height {
			changed = true
		} else {
			for x := 0; x < s.Width; x++ {
				if x >= prev.Width || s.cells[y][x] != prev.cells[y][x] {
					changed = true
					break
				}
			}
		}
		if !changed {
			continue
		}
		b.WriteString(fmt.Sprintf("\033[%d;%dH", y+1, 1))
		s.serializeRow(b, y)
	}
	b.WriteString(fmt.Sprintf("\033[%d;%dH", s.cy+1, s.cx+1))
}

func (s *Screen) serializeRow(b *strings.Builder, y int) {
	var currentAttr Attr
	var currentFg, currentBg Color
	var runes []rune
	flush := func() {
		if len(runes) > 0 {
			b.WriteString(string(runes))
			runes = runes[:0]
		}
	}
	emitAttr := func(attr Attr, fg, bg Color) {
		flush()
		b.WriteString("\033[")
		var parts []string
		if attr&AttrBold != 0 {
			parts = append(parts, "1")
		} else if currentAttr&AttrBold != 0 && attr&AttrBold == 0 {
			parts = append(parts, "22")
		}
		if attr&AttrDim != 0 {
			parts = append(parts, "2")
		} else if currentAttr&AttrDim != 0 && attr&AttrDim == 0 {
			parts = append(parts, "22")
		}
		if attr&AttrItalic != 0 {
			parts = append(parts, "3")
		} else if currentAttr&AttrItalic != 0 && attr&AttrItalic == 0 {
			parts = append(parts, "23")
		}
		if attr&AttrUnderline != 0 {
			parts = append(parts, "4")
		} else if currentAttr&AttrUnderline != 0 && attr&AttrUnderline == 0 {
			parts = append(parts, "24")
		}
		if attr&AttrBlink != 0 {
			parts = append(parts, "5")
		} else if currentAttr&AttrBlink != 0 && attr&AttrBlink == 0 {
			parts = append(parts, "25")
		}
		if attr&AttrReverse != 0 {
			parts = append(parts, "7")
		} else if currentAttr&AttrReverse != 0 && attr&AttrReverse == 0 {
			parts = append(parts, "27")
		}
		if attr&AttrStrike != 0 {
			parts = append(parts, "9")
		} else if currentAttr&AttrStrike != 0 && attr&AttrStrike == 0 {
			parts = append(parts, "29")
		}
		parts = append(parts, colorSeq(fg, currentFg, "38")...)
		parts = append(parts, colorSeq(bg, currentBg, "48")...)
		b.WriteString(strings.Join(parts, ";"))
		b.WriteString("m")
		currentAttr = attr
		currentFg = fg
		currentBg = bg
	}
	for x := 0; x < s.Width; x++ {
		cell := s.cells[y][x]
		if cell.Char == 0 {
			cell.Char = ' '
		}
		if cell.Attr != currentAttr || cell.Fg != currentFg || cell.Bg != currentBg {
			emitAttr(cell.Attr, cell.Fg, cell.Bg)
		}
		runes = append(runes, cell.Char)
	}
	flush()
	if currentAttr != 0 || !currentFg.IsDefault() || !currentBg.IsDefault() {
		b.WriteString("\033[m")
	}
}

func colorSeq(c, prev Color, prefix string) []string {
	if c == prev {
		return nil
	}
	if c.IsDefault() {
		return []string{prefix + ";0"}
	}
	switch c.Type {
	case ColorANSI:
		if c.Index < 8 {
			return []string{fmt.Sprintf("%s;5;%d", prefix, c.Index)}
		}
		return []string{fmt.Sprintf("%s;5;%d", prefix, c.Index)}
	case ColorANSI256:
		return []string{fmt.Sprintf("%s;5;%d", prefix, c.Index)}
	case ColorRGB:
		return []string{fmt.Sprintf("%s;2;%d;%d;%d", prefix, c.R, c.G, c.B)}
	}
	return nil
}

type Parser struct {
	Active        *Screen
	Normal        *Screen
	Alt           *Screen

	fg, bg       Color
	attr         Attr

	savedX, savedY int

	buf          []byte
	params       []int
	private      bool
	questionMark bool
	oscString    strings.Builder
	state        parserState
}

type parserState int

const (
	stateGround parserState = iota
	stateEscape
	stateCSI
	stateCSIParam
	stateCSIIntermediate
	stateOSC
	stateOSCString
	stateDCSEntry
	stateDCSString
)

func NewParser(w, h int) *Parser {
	p := &Parser{
		Normal: NewScreen(w, h),
		fg:     ColorDefaultFG,
		bg:     ColorDefaultBG,
		state:  stateGround,
	}
	p.Active = p.Normal
	return p
}

func (p *Parser) Feed(data []byte) {
	for _, b := range data {
		p.feedByte(b)
	}
}

func (p *Parser) feedByte(b byte) {
	switch p.state {
	case stateGround:
		p.ground(b)
	case stateEscape:
		p.escape(b)
	case stateCSI:
		p.csiEntry(b)
	case stateCSIParam:
		p.csiParam(b)
	case stateCSIIntermediate:
		p.csiIntermediate(b)
	case stateOSC:
		p.oscEntry(b)
	case stateOSCString:
		p.oscStringByte(b)
	case stateDCSEntry:
		p.dcsEntry(b)
	case stateDCSString:
		p.dcsStringByte(b)
	}
}

func (p *Parser) ground(b byte) {
	switch {
	case b == 0x1B:
		p.state = stateEscape
	case b == 0x00 || b == 0x7F:
	case b == 0x07:
	case b == 0x08:
		p.Active.Backspace()
	case b == 0x09:
		p.Active.Tab()
	case b == 0x0A:
		p.Active.LineFeed()
	case b == 0x0B:
		p.Active.LineFeed()
	case b == 0x0C:
		p.Active.LineFeed()
	case b == 0x0D:
		p.Active.CarriageReturn()
	case b == 0x0E:
	case b == 0x0F:
	case b >= 0x20 && b <= 0x7E:
		p.Active.PutRune(rune(b), p.fg, p.bg, p.attr)
	case b == 0x80:
	case b == 0x82:
	case b == 0x83:
	case b == 0x84:
		p.Active.LineFeed()
	case b == 0x85:
		p.Active.LineFeed()
	case b == 0x90:
		p.state = stateDCSEntry
	case b == 0x9B:
		p.state = stateCSI
		p.params = p.params[:0]
		p.private = false
		p.questionMark = false
	case b == 0x9D:
		p.state = stateOSC
		p.oscString.Reset()
	case b == 0x9E:
	case b == 0x9F:
	case b >= 0xA0:
		p.Active.PutRune(rune(b), p.fg, p.bg, p.attr)
	case b >= 0xC0 && b <= 0xDF:
		p.buf = append(p.buf[:0], b)
	case b >= 0xE0 && b <= 0xEF:
		p.buf = append(p.buf[:0], b)
	case b >= 0xF0 && b <= 0xF4:
		p.buf = append(p.buf[:0], b)
	default:
	}
}

func (p *Parser) escape(b byte) {
	p.state = stateGround
	switch {
	case b == '[':
		p.state = stateCSI
		p.params = p.params[:0]
		p.private = false
		p.questionMark = false
	case b == ']':
		p.state = stateOSC
		p.oscString.Reset()
	case b == 'P':
		p.state = stateDCSEntry
	case b == '^':
	case b == '_':
	case b == '7':
		p.savedX = p.Active.cx
		p.savedY = p.Active.cy
	case b == '8':
		p.Active.SetCursor(p.savedX, p.savedY)
	case b == 'c':
		w, h := p.Active.Width, p.Active.Height
		p.Normal = NewScreen(w, h)
		p.Alt = nil
		p.Active = p.Normal
		p.resetAttrs()
	case b == 'D':
		p.Active.Index()
	case b == 'E':
		p.Active.NextLine()
	case b == 'H':
		p.Active.SetTabStop()
	case b == 'M':
		p.Active.ReverseIndex()
	case b == 'Z':
		p.identifyTerminal()
	case b == '6':
		p.reportCursorPosition()
	case b == '=':
	case b == '>':
	case b == '(':
	case b == ')':
	case b == '*':
	case b == '+':
	case b == '/':
	case b == 'N':
	case b == 'O':
	case b == '#':
		p.state = stateGround
	default:
	}
}

func (p *Parser) identifyTerminal() {}

func (p *Parser) reportCursorPosition() {}

func (p *Parser) csiEntry(b byte) {
	switch {
	case b >= 0x30 && b <= 0x39:
		p.params = append(p.params, int(b-'0'))
		p.state = stateCSIParam
	case b == ';':
		p.params = append(p.params, -1)
		p.state = stateCSIParam
	case b == '<' || b == '=' || b == '>':
		p.private = true
	case b == '?':
		p.questionMark = true
	case b >= 0x20 && b <= 0x2F:
		p.buf = append(p.buf[:0], b)
		p.state = stateCSIIntermediate
	case b >= 0x40 && b <= 0x7E:
		p.executeCSI(b)
		p.state = stateGround
	default:
		p.state = stateGround
	}
}

func (p *Parser) csiParam(b byte) {
	switch {
	case b >= 0x30 && b <= 0x39:
		if len(p.params) > 0 {
			last := len(p.params) - 1
			if p.params[last] == -1 {
				p.params[last] = int(b - '0')
			} else {
				p.params[last] = p.params[last]*10 + int(b-'0')
			}
		}
	case b == ';':
		p.params = append(p.params, -1)
	case b >= 0x20 && b <= 0x2F:
		p.buf = append(p.buf[:0], b)
		p.state = stateCSIIntermediate
	case b >= 0x40 && b <= 0x7E:
		p.executeCSI(b)
		p.state = stateGround
	default:
		p.state = stateGround
	}
}

func (p *Parser) csiIntermediate(b byte) {
	switch {
	case b >= 0x20 && b <= 0x2F:
		p.buf = append(p.buf, b)
	case b >= 0x40 && b <= 0x7E:
		p.executeCSI(b)
		p.state = stateGround
	default:
		p.state = stateGround
	}
}

func (p *Parser) oscEntry(b byte) {
	p.state = stateOSCString
	if b != 0x1B {
		p.oscString.WriteByte(b)
	}
}

func (p *Parser) oscStringByte(b byte) {
	switch {
	case b == 0x07:
		p.executeOSC()
		p.state = stateGround
	case b == 0x1B:
		p.state = stateGround

	default:
		p.oscString.WriteByte(b)
	}
}

func (p *Parser) dcsEntry(b byte) {
	p.state = stateDCSString
}

func (p *Parser) dcsStringByte(b byte) {
	switch {
	case b == 0x1B:
		p.state = stateGround
	case b == 0x07:
		p.state = stateGround
	default:
	}
}

func (p *Parser) param(i int, def int) int {
	if i < 0 || i >= len(p.params) {
		return def
	}
	v := p.params[i]
	if v == -1 {
		return def
	}
	return v
}

func (p *Parser) paramOr(i int, def int) int {
	v := p.param(i, def)
	if v == 0 {
		return def
	}
	return v
}

func (p *Parser) executeCSI(b byte) {
	s := p.Active
	switch {
	case b == '@':
		s.InsertChars(p.paramOr(0, 1))
	case b == 'A':
		s.CursorUp(p.paramOr(0, 1))
	case b == 'B':
		s.CursorDown(p.paramOr(0, 1))
	case b == 'C':
		s.CursorForward(p.paramOr(0, 1))
	case b == 'D':
		s.CursorBackward(p.paramOr(0, 1))
	case b == 'E':
		s.CursorNextLine(p.paramOr(0, 1))
	case b == 'F':
		s.CursorPreviousLine(p.paramOr(0, 1))
	case b == 'G':
		s.HorizontalPositionAbsolute(p.paramOr(0, 1) - 1)
	case b == 'H':
		row := p.paramOr(0, 1) - 1
		col := p.paramOr(1, 1) - 1
		s.SetCursor(col, row)
	case b == 'I':
		for i := 0; i < p.paramOr(0, 1); i++ {
			s.Tab()
		}
	case b == 'J':
		s.EraseDisplay(p.param(0, 0))
	case b == 'K':
		s.EraseLine(p.param(0, 0))
	case b == 'L':
		s.InsertLines(p.paramOr(0, 1))
	case b == 'M':
		s.DeleteLines(p.paramOr(0, 1))
	case b == 'P':
		s.DeleteChars(p.paramOr(0, 1))
	case b == 'S':
		for i := 0; i < p.paramOr(0, 1); i++ {
			s.ScrollUp(1)
		}
	case b == 'T':
		for i := 0; i < p.paramOr(0, 1); i++ {
			s.ScrollDown(1)
		}
	case b == 'X':
		s.EraseChars(p.paramOr(0, 1))
	case b == 'd':
		s.VerticalPositionAbsolute(p.paramOr(0, 1) - 1)
	case b == 'f':
		row := p.paramOr(0, 1) - 1
		col := p.paramOr(1, 1) - 1
		s.SetCursor(col, row)
	case b == 'h':
		if p.questionMark {
			p.setPrivateMode(p.params, true)
		} else {
			p.setMode(p.params, true)
		}
	case b == 'l':
		if p.questionMark {
			p.setPrivateMode(p.params, false)
		} else {
			p.setMode(p.params, false)
		}
	case b == 'm':
		p.executeSGR(p.params)
	case b == 'r':
		top := p.param(0, 0) - 1
		bottom := p.param(1, 0) - 1
		if bottom < 0 {
			bottom = s.Height - 1
		}
		s.SetScrollRegion(top, bottom)
	case b == 's':
		p.savedX = s.cx
		p.savedY = s.cy
	case b == 'u':
		s.SetCursor(p.savedX, p.savedY)
	case b == 't':
		p.windowOp(p.params)
	case b == 'n':
		p.deviceStatusReport(p.params)
	default:
	}
}

func (p *Parser) setMode(params []int, set bool) {
	for _, v := range params {
		if v == -1 {
			continue
		}
		switch v {
		case 4:
			p.Active.insertMode = set
		}
	}
}

func (p *Parser) setPrivateMode(params []int, set bool) {
	s := p.Active
	val := 0
	if len(params) > 0 {
		val = params[0]
	}
	if val == -1 {
		val = 0
	}
	switch val {
	case 1:
		s.decCKM = set
	case 6:
		s.originMode = set
		if set {
			s.cx = 0
			s.cy = s.topMargin
		}
	case 7:
		s.autoWrap = set
	case 12:
	case 25:
		s.cursorVisible = set
	case 1047:
		p.switchAltScreen(set)
	case 1048:
	case 1049:
		p.switchAltScreen(set)
	case 2004:
		s.bracketedPaste = set
	}
}

func (p *Parser) switchAltScreen(enter bool) {
	if enter {
		if p.Alt == nil {
			alt := NewScreen(p.Normal.Width, p.Normal.Height)
			alt.autoWrap = p.Normal.autoWrap
			alt.cursorVisible = p.Normal.cursorVisible
			alt.decCKM = p.Normal.decCKM
			alt.bracketedPaste = p.Normal.bracketedPaste
			p.Alt = alt
		}
		p.Active = p.Alt
	} else {
		if p.Normal != nil {
			p.Normal.cursorVisible = p.Active.cursorVisible
			p.Normal.decCKM = p.Active.decCKM
			p.Normal.bracketedPaste = p.Active.bracketedPaste
		}
		p.Active = p.Normal
		p.Alt = nil
	}
}

func (p *Parser) executeSGR(params []int) {
	if len(params) == 0 || (len(params) == 1 && params[0] == -1) {
		p.resetAttrs()
		return
	}
	i := 0
	for i < len(params) {
		code := params[i]
		if code == -1 {
			code = 0
		}
		switch {
		case code == 0:
			p.resetAttrs()
		case code == 1:
			p.attr |= AttrBold
			p.attr &^= AttrDim
		case code == 2:
			p.attr |= AttrDim
			p.attr &^= AttrBold
		case code == 3:
			p.attr |= AttrItalic
		case code == 4:
			p.attr |= AttrUnderline
		case code == 5:
			p.attr |= AttrBlink
		case code == 7:
			p.attr |= AttrReverse
		case code == 9:
			p.attr |= AttrStrike
		case code == 22:
			p.attr &^= AttrBold
			p.attr &^= AttrDim
		case code == 23:
			p.attr &^= AttrItalic
		case code == 24:
			p.attr &^= AttrUnderline
		case code == 25:
			p.attr &^= AttrBlink
		case code == 27:
			p.attr &^= AttrReverse
		case code == 29:
			p.attr &^= AttrStrike
		case code >= 30 && code <= 37:
			p.fg = Color{Type: ColorANSI, Index: uint8(code - 30)}
		case code == 38:
			fg, next := parseExtendedColor(params, i)
			if fg != nil {
				p.fg = *fg
			}
			if next > i {
				i = next
			}
		case code == 39:
			p.fg = ColorDefaultFG
		case code >= 40 && code <= 47:
			p.bg = Color{Type: ColorANSI, Index: uint8(code - 40)}
		case code == 48:
			bg, next := parseExtendedColor(params, i)
			if bg != nil {
				p.bg = *bg
			}
			if next > i {
				i = next
			}
		case code == 49:
			p.bg = ColorDefaultBG
		case code == 90:
			p.fg = Color{Type: ColorANSI, Index: 8}
		case code == 91:
			p.fg = Color{Type: ColorANSI, Index: 9}
		case code == 92:
			p.fg = Color{Type: ColorANSI, Index: 10}
		case code == 93:
			p.fg = Color{Type: ColorANSI, Index: 11}
		case code == 94:
			p.fg = Color{Type: ColorANSI, Index: 12}
		case code == 95:
			p.fg = Color{Type: ColorANSI, Index: 13}
		case code == 96:
			p.fg = Color{Type: ColorANSI, Index: 14}
		case code == 97:
			p.fg = Color{Type: ColorANSI, Index: 15}
		case code == 100:
			p.bg = Color{Type: ColorANSI, Index: 8}
		case code == 101:
			p.bg = Color{Type: ColorANSI, Index: 9}
		case code == 102:
			p.bg = Color{Type: ColorANSI, Index: 10}
		case code == 103:
			p.bg = Color{Type: ColorANSI, Index: 11}
		case code == 104:
			p.bg = Color{Type: ColorANSI, Index: 12}
		case code == 105:
			p.bg = Color{Type: ColorANSI, Index: 13}
		case code == 106:
			p.bg = Color{Type: ColorANSI, Index: 14}
		case code == 107:
			p.bg = Color{Type: ColorANSI, Index: 15}
		}
		i++
	}
}

func (p *Parser) resetAttrs() {
	p.fg = ColorDefaultFG
	p.bg = ColorDefaultBG
	p.attr = 0
}

func parseExtendedColor(params []int, i int) (*Color, int) {
	if i+1 >= len(params) {
		return nil, i
	}
	sub := params[i+1]
	if sub == -1 {
		sub = 0
	}
	switch sub {
	case 2:
		if i+4 >= len(params) {
			return nil, i
		}
		r := params[i+2]
		g := params[i+3]
		b := params[i+4]
		if r < 0 { r = 0 }; if g < 0 { g = 0 }; if b < 0 { b = 0 }
		return &Color{Type: ColorRGB, R: uint8(r), G: uint8(g), B: uint8(b)}, i + 4
	case 5:
		if i+2 >= len(params) {
			return nil, i
		}
		idx := params[i+2]
		if idx < 0 { idx = 0 }
		return &Color{Type: ColorANSI256, Index: uint8(idx)}, i + 2
	}
	return nil, i
}

func (p *Parser) executeOSC() {
	s := p.oscString.String()
	parts := strings.SplitN(s, ";", 2)
	if len(parts) < 2 {
		return
	}
	switch parts[0] {
	case "0", "1", "2":
	case "4":
	case "8":
	case "10":
	case "11":
	case "12":
	}
}

func (p *Parser) windowOp(params []int) {}

func (p *Parser) deviceStatusReport(params []int) {}

func (p *Parser) ScreenSnapshot() string {
	return p.Active.Serialize(nil)
}

func (s *Screen) SetTabStop() {}

func (s *Screen) ClearTabStop() {}

func (s *Screen) ClearAllTabStops() {}
