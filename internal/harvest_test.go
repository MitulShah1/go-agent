package internal

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestHarvestTimer(t *testing.T) {
	now := time.Now()
	timer := newHarvestTimer(now, 10*time.Second)
	if ready := timer.ready(now.Add(9 * time.Second)); ready {
		t.Error(ready)
	}
	if ready := timer.ready(now.Add(11 * time.Second)); !ready {
		t.Error(ready)
	}
	if ready := timer.ready(now.Add(19 * time.Second)); ready {
		t.Error(ready)
	}
	if ready := timer.ready(now.Add(21 * time.Second)); !ready {
		t.Error(ready)
	}
	if ready := timer.ready(now.Add(29 * time.Second)); ready {
		t.Error(ready)
	}
	if ready := timer.ready(now.Add(31 * time.Second)); !ready {
		t.Error(ready)
	}
}

func TestCreateFinalMetrics(t *testing.T) {
	now := time.Now()

	// If the configurable harvest is nil then CreateFinalMetrics should
	// not panic.
	emptyHarvest := &Harvest{}
	emptyHarvest.CreateFinalMetrics(nil)

	var rules metricRules
	if err := json.Unmarshal([]byte(`[{
		"match_expression": "rename_me",
		"replacement": "been_renamed"
	}]`), &rules); nil != err {
		t.Fatal(err)
	}

	h := NewHarvest(now, nil)
	h.Metrics.addCount("rename_me", 1.0, unforced)
	h.CreateFinalMetrics(rules)
	ExpectMetrics(t, h.Metrics, []WantMetric{
		{instanceReporting, "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"been_renamed", "", false, []float64{1.0, 0, 0, 0, 0, 0}},
	})

	h = NewHarvest(now, nil)
	h.Metrics = newMetricTable(0, now)
	h.CustomEvents = newCustomEvents(1)
	h.TxnEvents = newTxnEvents(1)
	h.ErrorEvents = newErrorEvents(1)
	h.SpanEvents = newSpanEvents(1)

	h.SpanEvents.addEventPopulated(&sampleSpanEvent)
	h.SpanEvents.addEventPopulated(&sampleSpanEvent)

	customE, err := CreateCustomEvent("my event type", map[string]interface{}{"zip": 1}, time.Now())
	if nil != err {
		t.Fatal(err)
	}
	h.CustomEvents.Add(customE)
	h.CustomEvents.Add(customE)

	txnE := &TxnEvent{}
	h.TxnEvents.AddTxnEvent(txnE, 0)
	h.TxnEvents.AddTxnEvent(txnE, 0)

	h.ErrorEvents.Add(&ErrorEvent{}, 0)
	h.ErrorEvents.Add(&ErrorEvent{}, 0)

	h.CreateFinalMetrics(nil)
	ExpectMetrics(t, h.Metrics, []WantMetric{
		{instanceReporting, "", true, []float64{1, 0, 0, 0, 0, 0}},
	})
}

func TestEmptyPayloads(t *testing.T) {
	h := NewHarvest(time.Now(), nil)
	payloads := h.Payloads(true)
	if len(payloads) != 8 {
		t.Error(len(payloads))
	}
	for _, p := range payloads {
		d, err := p.Data("agentRunID", time.Now())
		if d != nil || err != nil {
			t.Error(d, err)
		}
	}
}

func TestPayloadsEmptyHarvest(t *testing.T) {
	h := &Harvest{}
	payloads := h.Payloads(true)
	if len(payloads) != 0 {
		t.Error(len(payloads))
	}
	var nilHarvest *Harvest
	payloads = nilHarvest.Payloads(true)
	if len(payloads) != 0 {
		t.Error(len(payloads))
	}
}

func payloadEndpointMethods(ps []PayloadCreator) map[string]struct{} {
	endpoints := make(map[string]struct{})
	for _, p := range ps {
		endpoints[p.EndpointMethod()] = struct{}{}
	}
	return endpoints
}

func TestHarvestNothingReady(t *testing.T) {
	now := time.Now()
	reply := ConnectReplyDefaults()
	reply.EventData = &harvestData{EventReportPeriodMs: 60000}
	h := NewHarvest(now, reply)
	fixedBefore := h.fixedHarvest
	configurableBefore := h.configurableHarvest
	ready := h.Ready(now.Add(10 * time.Second))
	payloads := ready.Payloads(true)
	if len(payloads) != 0 {
		t.Error(payloads)
	}
	if ready != nil {
		t.Error(ready)
	}
	ExpectMetrics(t, h.Metrics, []WantMetric{})
	if h.configurableHarvest != configurableBefore {
		t.Error(h.configurableHarvest, configurableBefore)
	}
	if h.fixedHarvest != fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
}

func TestConfigurableHarvestReady(t *testing.T) {
	now := time.Now()
	reply := ConnectReplyDefaults()
	reply.EventData = &harvestData{EventReportPeriodMs: 50000}
	h := NewHarvest(now, reply)
	fixedBefore := h.fixedHarvest
	configurableBefore := h.configurableHarvest
	ready := h.Ready(now.Add(51 * time.Second))
	payloads := ready.Payloads(true)
	endpoints := payloadEndpointMethods(payloads)
	if !reflect.DeepEqual(endpoints, map[string]struct{}{
		cmdCustomEvents: {},
		cmdTxnEvents:    {},
		cmdErrorEvents:  {},
	}) {
		t.Error(endpoints)
	}
	ExpectMetrics(t, h.Metrics, []WantMetric{
		{customEventsSeen, "", true, nil},
		{customEventsSent, "", true, nil},
		{txnEventsSeen, "", true, nil},
		{txnEventsSent, "", true, nil},
		{errorEventsSeen, "", true, nil},
		{errorEventsSent, "", true, nil},
	})
	if h.configurableHarvest == configurableBefore {
		t.Error(h.configurableHarvest, configurableBefore)
	}
	if ready.configurableHarvest != configurableBefore {
		t.Error(ready.configurableHarvest, configurableBefore)
	}
	if h.fixedHarvest != fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
	if ready.fixedHarvest != nil {
		t.Error(h.fixedHarvest)
	}
}

func TestFixedHarvestReady(t *testing.T) {
	now := time.Now()
	reply := ConnectReplyDefaults()
	reply.EventData = &harvestData{EventReportPeriodMs: 70000}
	h := NewHarvest(now, reply)
	fixedBefore := h.fixedHarvest
	configurableBefore := h.configurableHarvest
	ready := h.Ready(now.Add(61 * time.Second))
	payloads := ready.Payloads(true)
	endpoints := payloadEndpointMethods(payloads)
	if !reflect.DeepEqual(endpoints, map[string]struct{}{
		cmdMetrics:    {},
		cmdErrorData:  {},
		cmdTxnTraces:  {},
		cmdSlowSQLs:   {},
		cmdSpanEvents: {},
	}) {
		t.Error(endpoints)
	}
	ExpectMetrics(t, ready.Metrics, []WantMetric{
		{spanEventsSeen, "", true, nil},
		{spanEventsSent, "", true, nil},
	})
	if h.configurableHarvest != configurableBefore {
		t.Error(h.configurableHarvest, configurableBefore)
	}
	if ready.configurableHarvest != nil {
		t.Error(ready.configurableHarvest)
	}
	if h.fixedHarvest == fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
	if ready.fixedHarvest != fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
}

func TestFixedAndConfigurableReady(t *testing.T) {
	now := time.Now()
	reply := ConnectReplyDefaults()
	reply.EventData = &harvestData{EventReportPeriodMs: 60000}
	h := NewHarvest(now, reply)
	fixedBefore := h.fixedHarvest
	configurableBefore := h.configurableHarvest
	ready := h.Ready(now.Add(61 * time.Second))
	payloads := ready.Payloads(true)
	endpoints := payloadEndpointMethods(payloads)
	if !reflect.DeepEqual(endpoints, map[string]struct{}{
		cmdMetrics:      {},
		cmdCustomEvents: {},
		cmdTxnEvents:    {},
		cmdErrorEvents:  {},
		cmdErrorData:    {},
		cmdTxnTraces:    {},
		cmdSlowSQLs:     {},
		cmdSpanEvents:   {},
	}) {
		t.Error(endpoints)
	}
	ExpectMetrics(t, ready.Metrics, []WantMetric{
		{customEventsSeen, "", true, nil},
		{customEventsSent, "", true, nil},
		{txnEventsSeen, "", true, nil},
		{txnEventsSent, "", true, nil},
		{errorEventsSeen, "", true, nil},
		{errorEventsSent, "", true, nil},
		{spanEventsSeen, "", true, nil},
		{spanEventsSent, "", true, nil},
	})
	if h.configurableHarvest == configurableBefore {
		t.Error(h.configurableHarvest, configurableBefore)
	}
	if ready.configurableHarvest != configurableBefore {
		t.Error(ready.configurableHarvest, configurableBefore)
	}
	if h.fixedHarvest == fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
	if ready.fixedHarvest != fixedBefore {
		t.Error(h.fixedHarvest, fixedBefore)
	}
}

func TestMergeFailedHarvest(t *testing.T) {
	start1 := time.Now()
	start2 := start1.Add(1 * time.Minute)

	h := NewHarvest(start1, nil)
	h.Metrics.addCount("zip", 1, forced)
	h.TxnEvents.AddTxnEvent(&TxnEvent{
		FinalName: "finalName",
		Start:     time.Now(),
		Duration:  1 * time.Second,
		TotalTime: 2 * time.Second,
	}, 0)
	customEventParams := map[string]interface{}{"zip": 1}
	ce, err := CreateCustomEvent("myEvent", customEventParams, time.Now())
	if nil != err {
		t.Fatal(err)
	}
	h.CustomEvents.Add(ce)
	h.ErrorEvents.Add(&ErrorEvent{
		ErrorData: ErrorData{
			Klass: "klass",
			Msg:   "msg",
			When:  time.Now(),
		},
		TxnEvent: TxnEvent{
			FinalName: "finalName",
			Duration:  1 * time.Second,
		},
	}, 0)

	ers := NewTxnErrors(10)
	ers.Add(ErrorData{
		When:  time.Now(),
		Msg:   "msg",
		Klass: "klass",
		Stack: GetStackTrace(),
	})
	MergeTxnErrors(&h.ErrorTraces, ers, TxnEvent{
		FinalName: "finalName",
		Attrs:     nil,
	})
	h.SpanEvents.addEventPopulated(&sampleSpanEvent)

	if start1 != h.Metrics.metricPeriodStart {
		t.Error(h.Metrics.metricPeriodStart)
	}
	if 0 != h.Metrics.failedHarvests {
		t.Error(h.Metrics.failedHarvests)
	}
	if 0 != h.CustomEvents.events.failedHarvests {
		t.Error(h.CustomEvents.events.failedHarvests)
	}
	if 0 != h.TxnEvents.events.failedHarvests {
		t.Error(h.TxnEvents.events.failedHarvests)
	}
	if 0 != h.ErrorEvents.events.failedHarvests {
		t.Error(h.ErrorEvents.events.failedHarvests)
	}
	if 0 != h.SpanEvents.events.failedHarvests {
		t.Error(h.SpanEvents.events.failedHarvests)
	}
	ExpectMetrics(t, h.Metrics, []WantMetric{
		{"zip", "", true, []float64{1, 0, 0, 0, 0, 0}},
	})
	ExpectCustomEvents(t, h.CustomEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"type":      "myEvent",
			"timestamp": MatchAnything,
		},
		UserAttributes: customEventParams,
	}})
	ExpectErrorEvents(t, h.ErrorEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"error.class":     "klass",
			"error.message":   "msg",
			"transactionName": "finalName",
		},
	}})
	ExpectTxnEvents(t, h.TxnEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"name":      "finalName",
			"totalTime": 2.0,
		},
	}})
	ExpectSpanEvents(t, h.SpanEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"type":          "Span",
			"name":          "myName",
			"sampled":       true,
			"priority":      0.5,
			"category":      spanCategoryGeneric,
			"nr.entryPoint": true,
			"guid":          "guid",
			"transactionId": "txn-id",
			"traceId":       "trace-id",
		},
	}})
	ExpectErrors(t, h.ErrorTraces, []WantError{{
		TxnName: "finalName",
		Msg:     "msg",
		Klass:   "klass",
	}})

	nextHarvest := NewHarvest(start2, nil)
	if start2 != nextHarvest.Metrics.metricPeriodStart {
		t.Error(nextHarvest.Metrics.metricPeriodStart)
	}
	payloads := h.Payloads(true)
	for _, p := range payloads {
		p.MergeIntoHarvest(nextHarvest)
	}

	if start1 != nextHarvest.Metrics.metricPeriodStart {
		t.Error(nextHarvest.Metrics.metricPeriodStart)
	}
	if 1 != nextHarvest.Metrics.failedHarvests {
		t.Error(nextHarvest.Metrics.failedHarvests)
	}
	if 1 != nextHarvest.CustomEvents.events.failedHarvests {
		t.Error(nextHarvest.CustomEvents.events.failedHarvests)
	}
	if 1 != nextHarvest.TxnEvents.events.failedHarvests {
		t.Error(nextHarvest.TxnEvents.events.failedHarvests)
	}
	if 1 != nextHarvest.ErrorEvents.events.failedHarvests {
		t.Error(nextHarvest.ErrorEvents.events.failedHarvests)
	}
	if 1 != nextHarvest.SpanEvents.events.failedHarvests {
		t.Error(nextHarvest.SpanEvents.events.failedHarvests)
	}
	ExpectMetrics(t, nextHarvest.Metrics, []WantMetric{
		{"zip", "", true, []float64{1, 0, 0, 0, 0, 0}},
	})
	ExpectCustomEvents(t, nextHarvest.CustomEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"type":      "myEvent",
			"timestamp": MatchAnything,
		},
		UserAttributes: customEventParams,
	}})
	ExpectErrorEvents(t, nextHarvest.ErrorEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"error.class":     "klass",
			"error.message":   "msg",
			"transactionName": "finalName",
		},
	}})
	ExpectTxnEvents(t, nextHarvest.TxnEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"name":      "finalName",
			"totalTime": 2.0,
		},
	}})
	ExpectSpanEvents(t, h.SpanEvents, []WantEvent{{
		Intrinsics: map[string]interface{}{
			"type":          "Span",
			"name":          "myName",
			"sampled":       true,
			"priority":      0.5,
			"category":      spanCategoryGeneric,
			"nr.entryPoint": true,
			"guid":          "guid",
			"transactionId": "txn-id",
			"traceId":       "trace-id",
		},
	}})
	ExpectErrors(t, nextHarvest.ErrorTraces, []WantError{})
}

func TestCreateTxnMetrics(t *testing.T) {
	txnErr := &ErrorData{}
	txnErrors := []*ErrorData{txnErr}
	webName := "WebTransaction/zip/zap"
	backgroundName := "OtherTransaction/zip/zap"
	args := &TxnData{}
	args.Duration = 123 * time.Second
	args.TotalTime = 150 * time.Second
	args.ApdexThreshold = 2 * time.Second

	args.BetterCAT.Enabled = true

	args.FinalName = webName
	args.IsWeb = true
	args.Errors = txnErrors
	args.Zone = ApdexTolerating
	metrics := newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{webName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{webRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{dispatcherMetric, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"WebTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"WebTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"Errors/all", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/allWeb", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/" + webName, "", true, []float64{1, 0, 0, 0, 0, 0}},
		{apdexRollup, "", true, []float64{0, 1, 0, 2, 2, 0}},
		{"Apdex/zip/zap", "", false, []float64{0, 1, 0, 2, 2, 0}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/allWeb", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 0, 0, 0, 0, 0}},
		{"ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/allWeb", "", false, []float64{1, 0, 0, 0, 0, 0}},
	})

	args.FinalName = webName
	args.IsWeb = true
	args.Errors = nil
	args.Zone = ApdexTolerating
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{webName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{webRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{dispatcherMetric, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"WebTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"WebTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{apdexRollup, "", true, []float64{0, 1, 0, 2, 2, 0}},
		{"Apdex/zip/zap", "", false, []float64{0, 1, 0, 2, 2, 0}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/allWeb", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
	})

	args.FinalName = backgroundName
	args.IsWeb = false
	args.Errors = txnErrors
	args.Zone = ApdexNone
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{backgroundName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{backgroundRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"OtherTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"OtherTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"Errors/all", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/allOther", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/" + backgroundName, "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 0, 0, 0, 0, 0}},
		{"ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/allOther", "", false, []float64{1, 0, 0, 0, 0, 0}},
	})

	args.FinalName = backgroundName
	args.IsWeb = false
	args.Errors = nil
	args.Zone = ApdexNone
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{backgroundName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{backgroundRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"OtherTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"OtherTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
		{"DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", "", false, []float64{1, 123, 123, 123, 123, 123 * 123}},
	})

}

func TestHarvestSplitTxnEvents(t *testing.T) {
	now := time.Now()
	h := NewHarvest(now, nil)
	for i := 0; i < maxTxnEvents; i++ {
		h.TxnEvents.AddTxnEvent(&TxnEvent{}, Priority(float32(i)))
	}

	payloadsWithSplit := h.Payloads(true)
	payloadsWithoutSplit := h.Payloads(false)

	if len(payloadsWithSplit) != 9 {
		t.Error(len(payloadsWithSplit))
	}
	if len(payloadsWithoutSplit) != 8 {
		t.Error(len(payloadsWithoutSplit))
	}
}

func TestCreateTxnMetricsOldCAT(t *testing.T) {
	txnErr := &ErrorData{}
	txnErrors := []*ErrorData{txnErr}
	webName := "WebTransaction/zip/zap"
	backgroundName := "OtherTransaction/zip/zap"
	args := &TxnData{}
	args.Duration = 123 * time.Second
	args.TotalTime = 150 * time.Second
	args.ApdexThreshold = 2 * time.Second

	// When BetterCAT is disabled, affirm that the caller metrics are not created.
	args.BetterCAT.Enabled = false

	args.FinalName = webName
	args.IsWeb = true
	args.Errors = txnErrors
	args.Zone = ApdexTolerating
	metrics := newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{webName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{webRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{dispatcherMetric, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"WebTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"WebTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"Errors/all", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/allWeb", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/" + webName, "", true, []float64{1, 0, 0, 0, 0, 0}},
		{apdexRollup, "", true, []float64{0, 1, 0, 2, 2, 0}},
		{"Apdex/zip/zap", "", false, []float64{0, 1, 0, 2, 2, 0}},
	})

	args.FinalName = webName
	args.IsWeb = true
	args.Errors = nil
	args.Zone = ApdexTolerating
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{webName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{webRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{dispatcherMetric, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"WebTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"WebTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{apdexRollup, "", true, []float64{0, 1, 0, 2, 2, 0}},
		{"Apdex/zip/zap", "", false, []float64{0, 1, 0, 2, 2, 0}},
	})

	args.FinalName = backgroundName
	args.IsWeb = false
	args.Errors = txnErrors
	args.Zone = ApdexNone
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{backgroundName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{backgroundRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"OtherTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"OtherTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"Errors/all", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/allOther", "", true, []float64{1, 0, 0, 0, 0, 0}},
		{"Errors/" + backgroundName, "", true, []float64{1, 0, 0, 0, 0, 0}},
	})

	args.FinalName = backgroundName
	args.IsWeb = false
	args.Errors = nil
	args.Zone = ApdexNone
	metrics = newMetricTable(100, time.Now())
	CreateTxnMetrics(args, metrics)
	ExpectMetrics(t, metrics, []WantMetric{
		{backgroundName, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{backgroundRollup, "", true, []float64{1, 123, 0, 123, 123, 123 * 123}},
		{"OtherTransactionTotalTime", "", true, []float64{1, 150, 150, 150, 150, 150 * 150}},
		{"OtherTransactionTotalTime/zip/zap", "", false, []float64{1, 150, 150, 150, 150, 150 * 150}},
	})
}
