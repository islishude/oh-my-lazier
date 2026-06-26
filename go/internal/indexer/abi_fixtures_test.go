package indexer

import _ "embed"

//go:embed testdata/abis/packet_sent.json
var packetSentABIJSON string

//go:embed testdata/abis/sendlib_executor_fee_paid.json
var executorFeePaidABIJSON string

//go:embed testdata/abis/sendlib_dvn_fee_paid.json
var dvnFeePaidABIJSON string

//go:embed testdata/abis/open_executor_job_assigned.json
var executorJobAssignedABIJSON string

//go:embed testdata/abis/open_dvn_job_assigned.json
var dvnJobAssignedABIJSON string

//go:embed testdata/abis/endpoint_events.json
var endpointEventsABIJSON string
