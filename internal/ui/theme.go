package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// marineTheme — светлая «морская» тема v2.0: тёмно-синий акцент, мягкий
// фон, крупнее отступы и скругления. Остальное берётся из стандартной темы.
type marineTheme struct{}

var _ fyne.Theme = marineTheme{}

func (marineTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return color.NRGBA{0x1f, 0x5f, 0x9e, 0xff}
	case theme.ColorNameBackground:
		return color.NRGBA{0xf3, 0xf6, 0xfa, 0xff}
	case theme.ColorNameInputBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{0xe3, 0xea, 0xf2, 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{0x1b, 0x26, 0x33, 0xff}
	case theme.ColorNameSeparator:
		return color.NRGBA{0xd3, 0xdc, 0xe6, 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{0x1f, 0x5f, 0x9e, 0x33}
	}
	return theme.DefaultTheme().Color(n, theme.VariantLight)
}

func (marineTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }

func (marineTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (marineTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameText:
		return 15
	}
	return theme.DefaultTheme().Size(n)
}
