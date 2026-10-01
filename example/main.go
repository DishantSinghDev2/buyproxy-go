// Command example generates five US proxies and checks your exit IP through the first one.
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	buyproxy "github.com/DishantSinghDev2/buyproxy-go"
)

func main() {
	ctx := context.Background()
	client := buyproxy.New() // reads BUYPROXY_API_KEY

	plans, err := client.Plans.List(ctx)
	if err != nil || len(plans) == 0 {
		log.Fatalf("no plans yet — top up first: %v", err)
	}

	res, err := client.Plans.Generate(ctx, plans[0].ID, &buyproxy.GenerateParams{Country: "US", Count: 5})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.Lines)

	resp, err := res.Proxies[0].HTTPClient().Get("https://api.ipify.org")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	ip, _ := io.ReadAll(resp.Body)
	fmt.Println("exit IP:", string(ip))
}
