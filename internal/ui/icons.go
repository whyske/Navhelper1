package ui

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed icons/calculator.svg
var calculatorSVG []byte

// calculatorIcon — иконка раздела «Калькулятор» (калькулятор с дисплеем и
// клавишами). ThemedResource перекрашивает её под цвет темы, как стандартные.
var calculatorIcon fyne.Resource = theme.NewThemedResource(
	fyne.NewStaticResource("calculator.svg", calculatorSVG))
