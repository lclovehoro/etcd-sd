package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"regexp"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/klog/v2"
)

/*
查询所有的keys，或者以某个前缀的keys
etcdctl get --prefix ""
etcdctl get --prefix "/my-prefix"
只列出keys，不显示值
etcdctl get --prefix --keys-only ""
etcdctl get --prefix --keys-only "/my-prefix"
*/

// ./etcd_sd -target-file ./tgroups.json

type (
	instances map[string]string
	services  map[string]instances
)

type TargetGroup struct {
	Targets []string          `json:"targets,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

var (
	err            error
	targetFile     = flag.String("target-file", "tgroups.json", "the file that contains the target groups; default <tgroups.json>")
	dialTimeout    = 10 * time.Second
	etcdServer     = flag.String("server", "http://127.0.0.1:2379", "etcd server to connect to; default <http://127.0.0.1:2379>")
	servicesPrefix = flag.String("prefix", "/services", "performs a lookup of all services found in the <prefix> path and writes them out into a file of target groups; default <services>")
	cli            *clientv3.Client
	response       *clientv3.GetResponse
	s              = &services{}
	watchChan      = make(clientv3.WatchChan)
)

var pathPat = regexp.MustCompile(`/services/([^/]+)(?:/(\d+))?`)

func main() {
	flag.Parse()
	klog.InitFlags(nil)
	defer klog.Flush()

	cli, err = clientv3.New(clientv3.Config{
		//Endpoints:   endpoints,
		Endpoints:   []string{*etcdServer},
		DialTimeout: dialTimeout,
	})
	if err != nil {
		klog.Fatalf("Failed to connect to etcd server: %v", err)
	}
	defer cli.Close()
	klog.Info("Connected to etcd server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err = cli.Get(ctx, *servicesPrefix, clientv3.WithPrefix())
	if err != nil {
		klog.Fatalf("Failed to get data from path %s: %v", *servicesPrefix, err)
	}
	klog.Info("Data fetched from path:", *servicesPrefix)
	for _, kvs := range response.Kvs {
		s.handle(kvs, s.update)
	}

	//判断是否为null
	if len(*s) == 0 {
		klog.Infof("Empty; found in the %s path", *servicesPrefix)
	} else {
		s.persist()
	}

	go func() {
		for {
			klog.Infof("Start watching data from path: %s", *servicesPrefix)
			watchChan = cli.Watch(context.Background(), *servicesPrefix, clientv3.WithPrefix())
			for wresp := range watchChan {
				klog.Info("Received watch response")
				for _, ev := range wresp.Events {
					klog.Infof("Event received: %s %q : %q", ev.Type, ev.Kv.Key, ev.Kv.Value)
					handler := s.update
					if ev.Type == clientv3.EventTypeDelete {
						handler = s.delete
					}
					s.handle(ev.Kv, handler)
					s.persist()
				}
			}
			klog.Warning("Watch channel closed, reconnecting in 5s...")
			time.Sleep(5 * time.Second)
		}
	}()
	select {}
}

func (s services) handle(node *mvccpb.KeyValue, handler func(*mvccpb.KeyValue)) {
	if pathPat.MatchString(string(node.Key)) {
		handler(node)
	} else {
		klog.Warningf("Unhandled key %q", node.Key)
	}
}

func (s services) update(node *mvccpb.KeyValue) {
	match := pathPat.FindStringSubmatch(string(node.Key))
	if match[2] == "" {
		return
	}
	srv := match[1]
	instanceID := match[2]

	insts, ok := s[srv]
	if !ok {
		insts = instances{}
		s[srv] = insts
	}
	insts[instanceID] = string(node.Value)
}

func (s services) delete(node *mvccpb.KeyValue) {
	match := pathPat.FindStringSubmatch(string(node.Key))
	srv := match[1]
	instanceID := match[2]

	if instanceID == "" {
		delete(s, srv)
		return
	}

	delete(s[srv], instanceID)
}

func (s services) persist() {
	var (
		err     error
		content []byte
		tgroups []*TargetGroup
		file    io.WriteCloser
	)

	for job, instances := range s {
		targets := []string{}
		for _, addr := range instances {
			targets = append(targets, addr)
		}

		tgroups = append(tgroups, &TargetGroup{
			Targets: targets,
			Labels:  map[string]string{"job": job},
		})
	}

	if content, err = json.Marshal(tgroups); err != nil {
		klog.Error(err)
		return
	}

	if file, err = create(*targetFile); err != nil {
		klog.Error(err)
	}
	defer file.Close()

	if _, err = file.Write(content); err != nil {
		klog.Error(err)
	}
}
