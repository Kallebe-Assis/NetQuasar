package monitorworker

// OltCollectOutcome resultado de uma tentativa de coleta ONU/PON.
type OltCollectOutcome struct {
	OK       bool
	Skipped  bool
	Reason   string
	PonCount int
	Mode     string // if_derived | vendor_profile
}

