// «Личный штурманский помощник» — точка входа.
// Модули: калькулятор времени, календарь, каталог карт, погода, ИМ/НАВАРЕА/ПРИП.
package main

import (
	"fyne.io/fyne/v2/app"

	"ru.navhelper/internal/ui"
)

func main() {
	a := app.NewWithID("ru.navhelper.app")
	ui.Run(a)
}
