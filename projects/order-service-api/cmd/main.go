package main

import (
	"context"
	"fmt"
	"github.com/Waelson/lock-manager-service/order-service-api/pkg/sdk/locker"
	"time"
)

func main() {
	time.Sleep(10 * time.Second)
	lockClient := locker.NewLockClient("http://localhost:8181")
	ctx := context.Background()
	lock, releaseFunc, err := lockClient.Acquire(ctx, "job-file", "60s", "120s")

	if err != nil {
		panic("nao obteve o lock")
	}

	defer releaseFunc()

	fmt.Println("Lock: " + lock.String())

	time.Sleep(30 * time.Second)

	//err = lockClient.Release(ctx, lock)

	//if err != nil {
	//	panic("foi possivel liberar o lock")
	//}

}
