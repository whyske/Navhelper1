// warncheck — консольная проверка источников навигационных предупреждений
// и извещений мореплавателям (НАВАРЕА, ПРИП, НАВИП).
package main

import (
	"context"
	"fmt"
	"time"

	"ru.navhelper/internal/navwarn"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	list, errs := navwarn.NewClient().FetchAll(ctx)
	for _, e := range errs {
		fmt.Println("ошибка источника:", e)
	}
	byRegion := map[string]int{}
	for _, w := range list {
		byRegion[w.Region]++
	}
	fmt.Printf("всего предупреждений: %d\n", len(list))
	for k, v := range byRegion {
		fmt.Printf("  регион %-8s %d\n", k, v)
	}
	for i, w := range list {
		if i >= 10 {
			break
		}
		fmt.Printf("   %-24s | %s\n", w.ID(), w.Title)
	}
}
