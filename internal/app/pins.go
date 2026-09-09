package app

import (
	"context"
	"errors"
	"github.com/sausheong/harness/runtime"
)

func (c *Controller) withPins(ctx context.Context, change func([]runtime.ContextPin) ([]runtime.ContextPin, error)) ([]runtime.ContextPin, error) {
	operation, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return nil, err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil {
		return nil, errors.New("runtime unavailable")
	}
	pins, err := c.Rt.ContextPins(operation)
	if err != nil {
		return nil, err
	}
	if change == nil {
		return pins, nil
	}
	pins, err = change(pins)
	if err != nil {
		return nil, err
	}
	if err = c.Rt.SetContextPins(operation, pins); err != nil {
		return nil, err
	}
	return pins, nil
}
func (c *Controller) ContextPins(ctx context.Context) ([]runtime.ContextPin, error) {
	return c.withPins(ctx, nil)
}
func (c *Controller) SetContextPin(ctx context.Context, pin runtime.ContextPin) error {
	_, err := c.withPins(ctx, func(pins []runtime.ContextPin) ([]runtime.ContextPin, error) {
		for i := range pins {
			if pins[i].ID == pin.ID {
				pins[i] = pin
				return pins, nil
			}
		}
		return append(pins, pin), nil
	})
	return err
}
func (c *Controller) RemoveContextPin(ctx context.Context, id string) error {
	_, err := c.withPins(ctx, func(pins []runtime.ContextPin) ([]runtime.ContextPin, error) {
		for i := range pins {
			if pins[i].ID == id {
				return append(pins[:i], pins[i+1:]...), nil
			}
		}
		return nil, errors.New("context pin not found")
	})
	return err
}
