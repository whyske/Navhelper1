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

//go:embed icons/clock.svg
var clockSVG []byte

// clockIcon — иконка раздела «Часы» (циферблат со стрелками).
// Раньше здесь стояла theme.InfoIcon() — буква «i» в круге, которая к часам
// отношения не имеет. Перекрашивается темой так же, как стандартные.
var clockIconRes fyne.Resource = theme.NewThemedResource(
	fyne.NewStaticResource("clock.svg", clockSVG))

// clockIcon возвращает иконку раздела «Часы».
func clockIcon() fyne.Resource { return clockIconRes }
