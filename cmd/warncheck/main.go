// warncheck — консольная проверка источников предупреждений.
package main

import (
	"context"
	"fmt"
	"time"

	"ru.navhelper/internal/navwarn"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, r := range navwarn.NewClient().FetchAll(ctx, navwarn.DefaultSources) {
		fmt.Printf("%s: err=%v, предупреждений=%d\n", r.Source.Name, r.Err, len(r.Warnings))
		for i, w := range r.Warnings {
			if i < 3 {
				fmt.Printf("   %s | %s | отмена %v\n", w.ID(), w.Title, w.CancelAt)
			}
		}
	}
}
