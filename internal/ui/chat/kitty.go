package chat

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	rasterm "github.com/BourgeoisBear/rasterm"

	"github.com/ayn2op/discordo/internal/consts"
	"github.com/ayn2op/tview"
)

type kittyPreview struct {
	id      uint64
	pathB64 string
	cols    int
	rows    int

	loadOnce sync.Once
	loadErr  error
}

var (
	kittyWriteMu sync.Mutex
	kittyNextID  uint64
)

type messageListItem struct {
	*tview.TextView
	kittyPreview  *kittyPreview
	previewOffset int
}

func newMessageListItem(lines []tview.Line, preview *kittyPreview, previewOffset int) *messageListItem {
	tv := tview.NewTextView().
		SetWrap(true).
		SetWordWrap(true).
		SetLines(lines)
	return &messageListItem{
		TextView:      tv,
		kittyPreview:  preview,
		previewOffset: previewOffset,
	}
}

func (i *messageListItem) Draw(screen tcell.Screen) {
	i.TextView.Draw(screen)
	if i.kittyPreview == nil || i.previewOffset < 0 {
		return
	}

	x, y, width, height := i.InnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	if i.previewOffset >= height {
		return
	}

	cols := min(i.kittyPreview.cols, width)
	if cols <= 0 {
		return
	}

	rows := min(i.kittyPreview.rows, height-i.previewOffset)
	if rows <= 0 {
		return
	}

	startX := x
	startY := y + i.previewOffset
	maxX, maxY := screen.Size()
	if maxX <= 0 || maxY <= 0 {
		return
	}

	if startX >= maxX || startY >= maxY {
		return
	}
	if startX < 0 {
		cols += startX
		startX = 0
	}
	if startY < 0 {
		rows += startY
		startY = 0
	}
	if startX+cols > maxX {
		cols = maxX - startX
	}
	if startY+rows > maxY {
		rows = maxY - startY
	}
	if cols <= 0 || rows <= 0 {
		return
	}

	i.kittyPreview.drawAt(startX, startY, cols, rows)
}

func kittyDeleteAllPlacements() {
	kittyWrite(rasterm.KITTY_IMG_HDR + "a=d,d=a,q=2;" + rasterm.KITTY_IMG_FTR)
}

func kittyDeleteImageByID(id uint64) {
	if id == 0 {
		return
	}
	opts := rasterm.KittyImgOpts{ImageId: uint32(id)}
	kittyWrite(opts.ToHeader("a=d", "d=i", "q=2") + rasterm.KITTY_IMG_FTR)
}

func newKittyPreview(img image.Image, cols, rows int) (*kittyPreview, error) {
	if img == nil || cols <= 0 || rows <= 0 {
		return nil, nil
	}

	path, err := writeKittyImageFile(img)
	if err != nil {
		return nil, err
	}

	preview := &kittyPreview{
		id:      nextKittyImageID(),
		pathB64: base64.StdEncoding.EncodeToString([]byte(path)),
		cols:    cols,
		rows:    rows,
	}
	if err := preview.ensureLoaded(); err != nil {
		return nil, err
	}

	return preview, nil
}

func (p *kittyPreview) drawAt(x, y, cols, rows int) {
	if p == nil || p.pathB64 == "" || cols <= 0 || rows <= 0 {
		return
	}

	if x < 0 || y < 0 {
		// Never emit negative CSI H parameters; kitty rejects them.
		return
	}
	row := max(y+1, 1)
	col := max(x+1, 1)

	opts := rasterm.KittyImgOpts{
		ImageId: uint32(p.id),
		DstCols: uint32(cols),
		DstRows: uint32(rows),
	}

	var b strings.Builder
	b.WriteString("\x1b[s") // save cursor
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH", row, col))
	b.WriteString(opts.ToHeader("a=p", "q=2"))
	b.WriteString(rasterm.KITTY_IMG_FTR)
	b.WriteString("\x1b[u") // restore cursor
	kittyWrite(b.String())
}

func (p *kittyPreview) ensureLoaded() error {
	p.loadOnce.Do(func() {
		if p == nil || p.pathB64 == "" {
			return
		}
		opts := rasterm.KittyImgOpts{ImageId: uint32(p.id)}
		kittyWrite(opts.ToHeader("a=L", "f=100", "t=f", "q=2") + p.pathB64 + rasterm.KITTY_IMG_FTR)
	})

	return p.loadErr
}

func writeKittyImageFile(img image.Image) (string, error) {
	cacheDir := filepath.Join(consts.CacheDir(), "kitty")
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return "", err
	}

	file, err := os.CreateTemp(cacheDir, "preview-*.png")
	if err != nil {
		return "", err
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func nextKittyImageID() uint64 {
	return atomic.AddUint64(&kittyNextID, 1)
}

func kittyWrite(s string) {
	kittyWriteMu.Lock()
	defer kittyWriteMu.Unlock()
	_, _ = os.Stdout.WriteString(s)
}
