// Боковое выезжающее меню разделов вместо нижних вкладок.
// Готового drawer в Fyne нет, поэтому свой виджет: содержимое раздела,
// поверх него затемнение и панель, уезжающая влево за край экрана.
package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// navItem — один пункт меню.
type navItem struct {
	Name string
	Icon fyne.Resource
	View fyne.CanvasObject
}

// tapCatcher — прозрачный слой, закрывающий меню по нажатию.
type tapCatcher struct {
	widget.BaseWidget
	rect  *canvas.Rectangle
	onTap func()
}

func newTapCatcher() *tapCatcher {
	t := &tapCatcher{rect: canvas.NewRectangle(color.Transparent)}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapCatcher) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapCatcher) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.rect)
}

// navRoot — содержимое раздела + затемнение + панель меню.
type navRoot struct {
	widget.BaseWidget

	body    *fyne.Container
	overlay *tapCatcher
	drawer  *fyne.Container
	drawerW float32

	offset float32
	open   bool
	size   fyne.Size
}

func (n *navRoot) CreateRenderer() fyne.WidgetRenderer { return &navRootRenderer{n: n} }

type navRootRenderer struct{ n *navRoot }

func (r *navRootRenderer) Layout(size fyne.Size) {
	n := r.n
	n.size = size
	n.body.Move(fyne.NewPos(0, 0))
	n.body.Resize(size)
	n.overlay.Move(fyne.NewPos(0, 0))
	if n.open {
		n.overlay.Resize(size)
	} else {
		n.overlay.Resize(fyne.NewSize(0, 0))
	}
	n.drawer.Resize(fyne.NewSize(n.drawerW, size.Height))
	n.drawer.Move(fyne.NewPos(n.offset, 0))
}

func (r *navRootRenderer) MinSize() fyne.Size { return fyne.NewSize(240, 300) }
func (r *navRootRenderer) Refresh()           { r.Layout(r.n.size) }
func (r *navRootRenderer) Destroy()           {}

func (r *navRootRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.n.body, r.n.overlay, r.n.drawer}
}

// setOpen открывает или закрывает панель с анимацией.
func (n *navRoot) setOpen(open bool) {
	if open == n.open {
		return
	}
	start := n.offset
	end := float32(-n.drawerW)
	if open {
		end = 0
		n.overlay.Resize(n.size)
		n.overlay.Move(fyne.NewPos(0, 0))
	}
	n.open = open
	rect := n.overlay.rect
	anim := fyne.NewAnimation(200*time.Millisecond, func(t float32) {
		n.offset = start + (end-start)*t
		n.drawer.Move(fyne.NewPos(n.offset, 0))
		var alpha uint8
		if open {
			alpha = uint8(120 * t)
		} else {
			alpha = uint8(120 * (1 - t))
		}
		rect.FillColor = color.NRGBA{0, 0, 0, alpha}
		rect.Refresh()
		if !open && t >= 0.999 {
			n.overlay.Resize(fyne.NewSize(0, 0))
		}
	})
	anim.Curve = fyne.AnimationEaseInOut
	anim.Start()
}

// newNavRoot собирает навигацию: верхняя строка с гамбургером и названием
// раздела, под ней содержимое, слева выезжающая панель со списком разделов.
func newNavRoot(items []navItem, appName string) fyne.CanvasObject {
	n := &navRoot{drawerW: 270}
	n.ExtendBaseWidget(n)

	title := widget.NewLabelWithStyle(items[0].Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	page := container.NewStack(items[0].View)

	menu := widget.NewButtonWithIcon("", theme.MenuIcon(), func() { n.setOpen(true) })
	menu.Importance = widget.LowImportance
	topBar := container.NewBorder(nil, nil, menu, nil, title)
	n.body = container.NewBorder(container.NewPadded(topBar), widget.NewSeparator(), nil, nil, page)

	n.overlay = newTapCatcher()
	n.overlay.onTap = func() { n.setOpen(false) }

	header := canvas.NewRectangle(color.NRGBA{0x1f, 0x5f, 0x9e, 0xff})
	header.SetMinSize(fyne.NewSize(n.drawerW, 96))
	headerLabel := canvas.NewText(appName, color.White)
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}
	headerLabel.TextSize = theme.TextSize() * 1.5
	headerBox := container.NewStack(header,
		container.NewCenter(headerLabel))

	buttons := make([]*widget.Button, len(items))
	rows := container.NewVBox()
	for i, it := range items {
		i := i
		b := widget.NewButtonWithIcon(it.Name, it.Icon, func() {
			page.Objects = []fyne.CanvasObject{items[i].View}
			page.Refresh()
			title.SetText(items[i].Name)
			for j, bb := range buttons {
				if j == i {
					bb.Importance = widget.HighImportance
				} else {
					bb.Importance = widget.LowImportance
				}
				bb.Refresh()
			}
			n.setOpen(false)
		})
		b.Alignment = widget.ButtonAlignLeading
		b.Importance = widget.LowImportance
		buttons[i] = b
		rows.Add(b)
	}
	buttons[0].Importance = widget.HighImportance

	drawerBg := canvas.NewRectangle(color.NRGBA{0xf3, 0xf6, 0xfa, 0xff})
	n.drawer = container.NewStack(drawerBg, container.NewBorder(headerBox, nil, nil, nil,
		container.NewVScroll(container.NewPadded(rows))))
	n.offset = -n.drawerW

	// Корень: раздел на весь экран, поверх — затемнение и панель меню.
	return n
}
