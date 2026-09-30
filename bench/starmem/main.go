// Command starmem runs one memory attack against the compute engine with
// its default limits and reports how it ended (starmem-001). It is meant
// to run inside a memory-capped container: an OOM kill (exit 137) is the
// engine dying.
//
//	starmem [-isolated] <case>
//	starmem [-isolated] -latency 50     the median time of small-ok calls
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/metrics"
	"sort"
	"strings"
	"time"

	"github.com/bbiangul/tessera/internal/engines/compute"
)

// Cases are the attacks, each one function body.
var Cases = map[string]string{
	"loop-append":   "x = []\n    for i in range(100000000):\n        x.append('ab' * 1000)\n    return len(x)",
	"str-512m":      "return len('x' * (1 << 29))",
	"list-1g":       "return len([0] * (1 << 26))",
	"list-16g":      "return len([None] * ((1 << 30) - 1))",
	"concat-double": "s = 'x' * (1 << 28)\n    s = s + s\n    s = s + s\n    s = s + s\n    return len(s)",
	"comp-4g":       "return len([('x' * (1 << 29)) + str(i) for i in range(8)])",
	"join-4g":       "return len(''.join(['x' * (1 << 29)] * 8))",
	// Exploratory, after the pre-registered run: each stays under the
	// watchdog's 256 MiB until one instruction allocates gigabytes.
	"join-shared": "s = 'x' * (200 << 20)\n    return len(''.join([s] * 20))",
	"list-3g":     "return len([0] * (200 << 20))",
	"small-ok":    "return len([i * i for i in range(1000)])",
}

func main() {
	compute.RunWorkerIfRequested()
	isolated := flag.Bool("isolated", false, "run the call in a child process (compute.Isolate)")
	latency := flag.Int("latency", 0, "time this many small-ok calls and print the median")
	flag.Parse()
	if *isolated {
		compute.Isolate(&compute.Isolation{})
	}
	if *latency > 0 {
		timeCalls(*latency)
		return
	}
	name := flag.Arg(0)
	body, ok := Cases[name]
	if !ok {
		fmt.Fprintln(os.Stderr, "unknown case")
		os.Exit(2)
	}
	s, err := compute.Compile("a.star", []byte("def f():\n    "+body+"\n"), compute.Limits{})
	if err != nil {
		fmt.Println("compile:", err)
		os.Exit(3)
	}
	start := time.Now()
	out, err := s.Call(context.Background(), "f", nil, compute.Limits{})
	sample := []metrics.Sample{{Name: "/memory/classes/total:bytes"}}
	metrics.Read(sample)
	res := fmt.Sprint(out)
	if err != nil {
		res = "error: " + strings.TrimPrefix(err.Error(), "compute failed: ")
	}
	fmt.Printf("%s\t%s\t%v\tgo-mem=%dMiB\n", name, res, time.Since(start).Round(time.Millisecond), sample[0].Value.Uint64()>>20)
}

func timeCalls(n int) {
	s, err := compute.Compile("a.star", []byte("def f():\n    "+Cases["small-ok"]+"\n"), compute.Limits{})
	if err != nil {
		fmt.Println("compile:", err)
		os.Exit(3)
	}
	ds := make([]time.Duration, n)
	for i := range ds {
		start := time.Now()
		if _, err := s.Call(context.Background(), "f", nil, compute.Limits{}); err != nil {
			fmt.Println("call:", err)
			os.Exit(3)
		}
		ds[i] = time.Since(start)
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	fmt.Printf("median=%v p90=%v\n", ds[n/2], ds[n*9/10])
}
