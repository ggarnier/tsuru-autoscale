// Copyright 2015 tsuru-autoscale authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package alarm

import (
	"errors"
	"time"

	"github.com/tsuru/tsuru-autoscale/action"
	"github.com/tsuru/tsuru/log"
	"gopkg.in/mgo.v2"
)

func StartAutoScale() {
	go runAutoScale()
}

// Config represents the configuration for the auto scale.
type Config struct {
	Name     string        `json:"name"`
	Increase action.Action `json:"increase"`
	Decrease action.Action `json:"decrease"`
	MinUnits uint          `json:"minUnits"`
	MaxUnits uint          `json:"maxUnits"`
	Enabled  bool          `json:"enabled"`
}

func runAutoScaleOnce() {
	configs := []Config{}
	for _, config := range configs {
		err := scaleIfNeeded(&config)
		if err != nil {
			log.Error(err.Error())
		}
	}
}

func runAutoScale() {
	for {
		runAutoScaleOnce()
		time.Sleep(30 * time.Second)
	}
}

func scaleIfNeeded(config *Config) error {
	if config == nil {
		return errors.New("AutoScale is not configured.")
	}
	/*
		increaseMetric, _ := app.Metric(config.Increase.metric())
		value, _ := config.Increase.value()
		if increaseMetric > value {
			currentUnits := uint(len(app.Units()))
			maxUnits := config.MaxUnits
			if maxUnits == 0 {
				maxUnits = 1
			}
			if currentUnits >= maxUnits {
				return nil
			}
			if wait, err := shouldWait(app, config.Increase.Wait); err != nil {
				return err
			} else if wait {
				return nil
			}
			evt, err := NewEvent(app, "increase")
			if err != nil {
				return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
		 	}
			inc := config.Increase.Units
			if currentUnits+inc > config.MaxUnits {
				inc = config.MaxUnits - currentUnits
			}
			addUnitsErr := app.AddUnits(inc, nil)
			err = evt.update(addUnitsErr)
			if err != nil {
				log.Errorf("Error trying to update auto scale event: %s", err.Error())
			}
			return addUnitsErr
		}
		decreaseMetric, _ := app.Metric(config.Decrease.metric())
		value, _ = config.Decrease.value()
		if decreaseMetric < value {
			currentUnits := uint(len(app.Units()))
			minUnits := config.MinUnits
			if minUnits == 0 {
				minUnits = 1
			}
			if currentUnits <= minUnits {
				return nil
			}
			if wait, err := shouldWait(app, config.Decrease.Wait); err != nil {
				return err
			} else if wait {
				return nil
			}
			evt, err := NewEvent(app, "decrease")
			if err != nil {
				return fmt.Errorf("Error trying to insert auto scale event, auto scale aborted: %s", err.Error())
			}
			dec := config.Decrease.Units
			if currentUnits-dec < config.MinUnits {
				dec = currentUnits - config.MinUnits
			}
			removeUnitsErr := app.RemoveUnits(dec)
			err = evt.update(removeUnitsErr)
			if err != nil {
				log.Errorf("Error trying to update auto scale event: %s", err.Error())
			}
			return removeUnitsErr
		}
	*/
	return nil
}

func shouldWait(config *Config, waitPeriod time.Duration) (bool, error) {
	now := time.Now().UTC()
	lastEvent, err := lastScaleEvent(config)
	if err != nil && err != mgo.ErrNotFound {
		return false, err
	}
	if err != mgo.ErrNotFound && lastEvent.EndTime.IsZero() {
		return true, nil
	}
	diff := now.Sub(lastEvent.EndTime)
	if diff > waitPeriod {
		return false, nil
	}
	return true, nil
}

func AutoScaleEnable(config *Config) error {
	config.Enabled = true
	return nil
}

func AutoScaleDisable(config *Config) error {
	config.Enabled = false
	return nil
}