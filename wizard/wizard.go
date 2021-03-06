// Copyright 2017 tsuru-autoscale authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wizard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tsuru/tsuru-autoscale/alarm"
	"github.com/tsuru/tsuru-autoscale/datasource"
	"github.com/tsuru/tsuru-autoscale/db"
	"github.com/tsuru/tsuru-autoscale/log"
	"gopkg.in/mgo.v2/bson"
)

var (
	unitsExpression   = `!units.lock.Locked && units.units.map(function(unit){ if (unit.ProcessName === "{process}") {return 1} else {return 0}}).reduce(function(c, p) { return c + p }) > {minUnits}`
	defaultExpression = `{metric}.aggregations.range.buckets[0].date.buckets[{metric}.aggregations.range.buckets[0].date.buckets.length - 1].{aggregator}.value {operator} {value}`
)

func logger() *log.Logger {
	return log.Log()
}

// AutoScale represents a auto scale configuration
type AutoScale struct {
	Name      string      `json:"name"`
	ScaleUp   ScaleAction `json:"scaleUp"`
	ScaleDown ScaleAction `json:"scaleDown"`
	MinUnits  int         `json:"minUnits"`
	Process   string      `json:"process"`
}

// MarshalJSON marshals AutoScale in json format
func (a *AutoScale) MarshalJSON() ([]byte, error) {
	type alias AutoScale
	return json.Marshal(&struct {
		Enabled bool `json:"enabled"`
		*alias
	}{
		Enabled: a.Enabled(),
		alias:   (*alias)(a),
	})
}

// ScaleAction represents a auto scale action like scale up or scale down.
type ScaleAction struct {
	Aggregator string        `json:"aggregator"`
	Metric     string        `json:"metric"`
	Operator   string        `json:"operator"`
	Value      string        `json:"value"`
	Step       string        `json:"step"`
	Wait       time.Duration `json:"wait"`
}

// New creates a new auto scale based on AutoScale configuration
func New(a *AutoScale) error {
	if a.MinUnits <= 0 {
		a.MinUnits = 1
	}
	err := newScaleAction(a, "scale_up")
	if err != nil {
		logger().Error(err)
		return err
	}
	err = newScaleAction(a, "scale_down")
	if err != nil {
		logger().Error(err)
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		logger().Error(err)
		return nil
	}
	defer conn.Close()
	return conn.Wizard().Insert(&a)
}

func newScaleAction(scaleConfig *AutoScale, kind string) error {
	var (
		name        string
		processName string
		action      ScaleAction
		datasources []string
	)
	if kind == "scale_up" {
		action = scaleConfig.ScaleUp
		datasources = []string{action.Metric}
	}
	if kind == "scale_down" {
		action = scaleConfig.ScaleDown
		datasources = []string{"units", action.Metric}
	}
	if scaleConfig.Process == "" {
		name = fmt.Sprintf("%s_%s", kind, scaleConfig.Name)
		processName = "web"
	} else {
		name = fmt.Sprintf("%s_%s_%s", kind, scaleConfig.Name, scaleConfig.Process)
		processName = scaleConfig.Process
	}
	aggregator := action.Aggregator
	if aggregator == "" {
		aggregator = "max"
	}
	var expParts []string
	for _, d := range datasources {
		ds, _ := datasource.Get(d)
		if ds == nil || ds.ExpressionTemplate == "" {
			if d == "units" {
				expParts = append(expParts, unitsExpression)
			} else {
				expParts = append(expParts, defaultExpression)
			}
		} else {
			expParts = append(expParts, ds.ExpressionTemplate)
		}
	}
	expression := strings.Join(expParts, " && ")
	replacer := strings.NewReplacer(
		"{aggregator}", aggregator,
		"{operator}", action.Operator,
		"{value}", action.Value,
		"{minUnits}", strconv.Itoa(scaleConfig.MinUnits),
		"{metric}", action.Metric,
	)
	envs := map[string]string{
		"step":       action.Step,
		"process":    processName,
		"aggregator": aggregator,
	}
	a := alarm.Alarm{
		Name:        name,
		Expression:  replacer.Replace(expression),
		Enabled:     true,
		Wait:        action.Wait * time.Second,
		Actions:     []string{kind},
		Instance:    scaleConfig.Name,
		DataSources: datasources,
		Envs:        envs,
	}
	return alarm.NewAlarm(&a)
}

// FindByfinds auto scale by a query "q"
func FindBy(q bson.M) ([]AutoScale, error) {
	conn, err := db.Conn()
	if err != nil {
		logger().Error(err)
		return nil, err
	}
	defer conn.Close()
	var a []AutoScale
	err = conn.Wizard().Find(q).All(&a)
	if err != nil {
		logger().Error(err)
		return nil, err
	}
	return a, nil
}

// FindByName finds auto scale by name
func FindByName(name string) (*AutoScale, error) {
	l, err := FindBy(bson.M{"name": name})
	if err != nil {
		logger().Error(err)
		return nil, err
	}
	if len(l) > 0 {
		return &l[0], nil
	}
	return nil, fmt.Errorf("wizard %q not found", name)
}

func (a *AutoScale) alarms() []string {
	var alarms []string
	if a.Process == "" {
		alarms = append(alarms, fmt.Sprintf("scale_up_%s", a.Name))
		alarms = append(alarms, fmt.Sprintf("scale_down_%s", a.Name))
	} else {
		alarms = append(alarms, fmt.Sprintf("scale_up_%s_%s", a.Name, a.Process))
		alarms = append(alarms, fmt.Sprintf("scale_down_%s_%s", a.Name, a.Process))
	}
	return alarms
}

func removeAlarms(autoScale *AutoScale) error {
	for _, a := range autoScale.alarms() {
		al, err := alarm.FindAlarmByName(a)
		if err != nil {
			logger().Error(err)
			return err
		}
		err = alarm.RemoveAlarm(al)
		if err != nil {
			logger().Error(err)
			return err
		}
	}
	return nil
}

// Remove removes an auto scale.
func Remove(a *AutoScale) error {
	err := removeAlarms(a)
	if err != nil {
		logger().Error(err)
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		logger().Error(err)
		return err
	}
	defer conn.Close()
	return conn.Wizard().Remove(a)
}

// Events return a list of AutoScale events
func (a *AutoScale) Events() ([]alarm.Event, error) {
	conn, err := db.Conn()
	if err != nil {
		logger().Error(err)
		return nil, err
	}
	defer conn.Close()
	var events []alarm.Event
	q := bson.M{"alarm.instance": a.Name, "alarm.actions": bson.M{"$in": []string{"scale_up", "scale_down"}}}
	err = conn.Events().Find(q).Sort("-starttime").Limit(200).All(&events)
	if err != nil {
		logger().Error(err)
		return nil, err
	}
	return events, nil
}

// Enable enables the AutoScale alarms
func (a *AutoScale) Enable() error {
	for _, alarmName := range a.alarms() {
		al, err := alarm.FindAlarmByName(alarmName)
		if err != nil {
			return err
		}
		err = alarm.Enable(al)
		if err != nil {
			return err
		}
	}
	return nil
}

// Disable disables the AutoScale alarms
func (a *AutoScale) Disable() error {
	for _, alarmName := range a.alarms() {
		al, err := alarm.FindAlarmByName(alarmName)
		if err != nil {
			return err
		}
		err = alarm.Disable(al)
		if err != nil {
			return err
		}
	}
	return nil
}

// Enabled returns true if the AutoScale alarms are enabled
func (a *AutoScale) Enabled() bool {
	for _, alarmName := range a.alarms() {
		al, err := alarm.FindAlarmByName(alarmName)
		if err != nil {
			return false
		}
		if !al.Enabled {
			return false
		}
	}
	return true
}

// Update updates an auto scale
func Update(a *AutoScale) error {
	old, err := FindByName(a.Name)
	if err != nil {
		return err
	}
	if a.MinUnits <= 0 {
		a.MinUnits = 1
	}
	err = removeAlarms(old)
	if err != nil {
		return err
	}
	err = newScaleAction(a, "scale_up")
	if err != nil {
		logger().Error(err)
		return err
	}
	err = newScaleAction(a, "scale_down")
	if err != nil {
		logger().Error(err)
		return err
	}
	conn, err := db.Conn()
	if err != nil {
		logger().Error(err)
		return nil
	}
	defer conn.Close()
	return conn.Wizard().Update(bson.M{"name": a.Name}, a)
}
