// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// cache implements base vm factory on top of other base vm factory.

package cache

import (
	"context"
	"fmt"
	"sync"

	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/factory/base"
)

type cache struct {
	base base.FactoryBase

	cacheCh   chan *vc.VM
	closed    chan<- int
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// New creates a new cached vm factory.
func New(ctx context.Context, count uint, b base.FactoryBase) base.FactoryBase {
	if count < 1 {
		return b
	}

	cacheCh := make(chan *vc.VM)
	closed := make(chan int, count)
	c := cache{base: b, cacheCh: cacheCh, closed: closed}
	for i := 0; i < int(count); i++ {
		c.wg.Add(1)
		go func() {
			for {
				vm, err := b.GetBaseVM(ctx, c.Config())
				if err != nil {
					c.wg.Done()
					c.CloseFactory(ctx)
					return
				}

				select {
				case cacheCh <- vm:
				case <-closed:
					vm.Stop()
					c.wg.Done()
					return
				}
			}
		}()
	}
	return &c
}

// Config returns cache vm factory's base factory config.
func (c *cache) Config() vc.VMConfig {
	return c.base.Config()
}

// GetBaseVM returns a base VM from cache factory's base factory.
func (c *cache) GetBaseVM(ctx context.Context, config vc.VMConfig) (*vc.VM, error) {
	vm, ok := <-c.cacheCh
	if ok {
		return vm, nil
	}
	return nil, fmt.Errorf("cache factory is closed")
}

// CloseFactory closes the cache factory.
func (c *cache) CloseFactory(ctx context.Context) {
	c.closeOnce.Do(func() {
		for len(c.closed) < cap(c.closed) { // send sufficient closed signal
			c.closed <- 0
		}
		c.wg.Wait()
		close(c.cacheCh)
		c.base.CloseFactory(ctx)
	})
}
