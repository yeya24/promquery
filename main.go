package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

const (
	queryCPU              = `sum(kube_node_status_capacity{cluster="%s", resource="cpu"}) * 1000`
	queryMemory           = `sum(kube_node_status_capacity{cluster="%s", resource="memory"})/1024/1024`
	queryEphemeralStorage = `sum(node_filesystem_size_bytes{fstype=~"ext[234]|btrfs|xfs|zfs",cluster="%s", job="node-exporter", mountpoint="/var"} )/1024/1024/1024`
	queryStorage          = `ceph_cluster_total_bytes{cluster="%s"}/1024/1024/1024`
)

var (
	cluster     = flag.String("cluster", "", "cluster name to fetch resource info")
	thanosQuery = flag.String("thanos", "", "thanos query URL")
	timeout     = flag.Duration("timeout", time.Second*10, "timeout of each thanos query")
)

func main() {
	flag.Parse()
	if *cluster == "" || *thanosQuery == "" {
		fmt.Printf("cluster or thanos query url cannot be empty")
		os.Exit(1)
	}

	if err := run(); err != nil {
		fmt.Printf("failed to query thanos: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	c, err := api.NewClient(api.Config{Address: *thanosQuery})
	if err != nil {
		return err
	}

	a := v1.NewAPI(c)
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var v int64
	items := []string{"CPU", "memory", "ephemeral storage", "storage"}
	for i, q := range []string{queryCPU, queryMemory, queryEphemeralStorage, queryStorage} {
		v, err = query(ctx, a, now, q, *cluster)
		if err != nil {
			return err
		}
		fmt.Printf("%s cluster %s: %d\n", *cluster, items[i], v)
	}
	return nil
}

func query(ctx context.Context, api v1.API, t time.Time, query, cluster string) (int64, error) {
	var (
		v   model.Value
		err error
	)
	v, _, err = api.Query(ctx, fmt.Sprintf(query, cluster), t)
	if err != nil {
		return 0, err
	}

	return getValue(v)
}

func getValue(val model.Value) (int64, error) {
	v := val.(model.Vector)
	if v.Len() == 0 {
		return 0, errors.New("empty result returned, please check whether the cluster is added to Thanos query or not")
	}
	return int64(v[0].Value), nil
}
