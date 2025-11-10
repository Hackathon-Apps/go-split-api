package metrics

import "github.com/prometheus/client_golang/prometheus"

const namespace = "split"

type Metrics struct {
	reg prometheus.Registerer

	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec

	WsConnections prometheus.Gauge

	BillsCreated          prometheus.Counter
	TransactionsCreated   *prometheus.CounterVec
	TransactionsFinalized *prometheus.CounterVec
	TxWatchersActive      prometheus.Gauge
	TxWatcherDuration     *prometheus.HistogramVec

	TonStreamConnected   prometheus.Gauge
	TonStreamConnections prometheus.Counter
	TonStreamEvents      *prometheus.CounterVec
	TonStreamSubscribes  *prometheus.CounterVec

	TonCenterRequests        *prometheus.CounterVec
	TonCenterRequestDuration *prometheus.HistogramVec

	BillAutoTimeouts *prometheus.CounterVec
}

func New(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		reg: reg,
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total amount of processed HTTP requests.",
		}, []string{"route", "method", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Request processing latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
		WsConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ws_connections",
			Help:      "Current amount of active WebSocket connections.",
		}),
		BillsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bills_created_total",
			Help:      "Amount of bills created via API.",
		}),
		TransactionsCreated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transactions_created_total",
			Help:      "Amount of transactions created by op type.",
		}, []string{"op"}),
		TransactionsFinalized: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transactions_finalized_total",
			Help:      "Amount of transactions finalized by result.",
		}, []string{"result"}),
		TxWatchersActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tx_watchers_active",
			Help:      "Current amount of active transaction watchers.",
		}),
		TxWatcherDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "tx_watcher_duration_seconds",
			Help:      "How long transaction watchers run until completion.",
			Buckets:   []float64{1, 5, 15, 30, 60, 120, 300, 600, 900},
		}, []string{"result"}),
		TonStreamConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tonstream_connected",
			Help:      "Whether the TonStream websocket is connected (1) or not (0).",
		}),
		TonStreamConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tonstream_connections_total",
			Help:      "Amount of successful websocket (re)connections to TonAPI.",
		}),
		TonStreamEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tonstream_events_total",
			Help:      "TonStream events grouped by outcome.",
		}, []string{"type"}),
		TonStreamSubscribes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tonstream_subscribes_total",
			Help:      "TonStream subscribe attempts grouped by result.",
		}, []string{"result"}),
		TonCenterRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "toncenter_requests_total",
			Help:      "TonCenter getTransactions requests grouped by result.",
		}, []string{"result"}),
		TonCenterRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "toncenter_request_duration_seconds",
			Help:      "TonCenter request latency.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
		}, []string{"result"}),
		BillAutoTimeouts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bill_auto_timeouts_total",
			Help:      "Automatic bill timeout attempts grouped by result.",
		}, []string{"result"}),
	}

	reg.MustRegister(
		m.HTTPRequests,
		m.HTTPDuration,
		m.WsConnections,
		m.BillsCreated,
		m.TransactionsCreated,
		m.TransactionsFinalized,
		m.TxWatchersActive,
		m.TxWatcherDuration,
		m.TonStreamConnected,
		m.TonStreamConnections,
		m.TonStreamEvents,
		m.TonStreamSubscribes,
		m.TonCenterRequests,
		m.TonCenterRequestDuration,
		m.BillAutoTimeouts,
	)

	return m
}
