package transcdr

import "time"

// UsageParams choose a usage window.
type UsageParams struct {
	// From and To are YYYY-MM-DD or RFC 3339.
	From string
	To   string
	// Granularity is day, week or month.
	Granularity string
}

// UsagePoint is one period of usage.
type UsagePoint struct {
	Date            string  `json:"date"`
	Jobs            int64   `json:"jobs"`
	BillableMinutes float64 `json:"billable_minutes"`
	AmountCents     int64   `json:"amount_cents"`
	AmountUSD       float64 `json:"amount_usd"`
}

// UsageTotals are the window's totals.
type UsageTotals struct {
	Jobs            int64   `json:"jobs"`
	BillableMinutes float64 `json:"billable_minutes"`
	InputMinutes    float64 `json:"input_minutes"`
	OutputBytes     int64   `json:"output_bytes"`
	AmountCents     int64   `json:"amount_cents"`
	AmountUSD       float64 `json:"amount_usd"`
}

// Usage is usage over a window.
type Usage struct {
	From        string             `json:"from"`
	To          string             `json:"to"`
	Granularity string             `json:"granularity"`
	Totals      UsageTotals        `json:"totals"`
	ByTier      map[string]float64 `json:"by_tier"`
	ByCodec     map[string]float64 `json:"by_codec"`
	Series      []UsagePoint       `json:"series"`
}

// RateCard is the price per output minute, in dollars.
type RateCard struct {
	Unit     string  `json:"unit"`
	Currency string  `json:"currency"`
	SD       float64 `json:"sd"`
	HD       float64 `json:"hd"`
	UHD      float64 `json:"uhd"`
	// Tiers say what each tier covers, e.g. "577p to 1440p".
	Tiers map[string]string `json:"tiers"`
}

// Plan is a plan. The unlimited plan is never listed and cannot be bought.
type Plan struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Tagline string `json:"tagline,omitempty"`
	// Subscription plans are bought monthly through checkout.
	Subscription bool `json:"subscription"`
	// PriceCents is monthly; nil means "contact us".
	PriceCents         *int64   `json:"price_cents"`
	Currency           string   `json:"currency,omitempty"`
	MonthlyCreditCents int64    `json:"monthly_credit_cents"`
	CreditValueRatio   *float64 `json:"credit_value_ratio"`
	TrialCreditCents   int64    `json:"trial_credit_cents"`
	TrialDays          int      `json:"trial_days"`
	Rates              RateCard `json:"rates"`
	MaxConcurrentJobs  int      `json:"max_concurrent_jobs"`
	MaxResolution      int      `json:"max_resolution"`
	MaxInputBytes      *int64   `json:"max_input_bytes,omitempty"`
	Priority           bool     `json:"priority"`
	RetentionDays      int      `json:"retention_days"`
	RequestsPerMinute  *int     `json:"requests_per_minute,omitempty"`
	Features           []string `json:"features"`
}

// AutoRecharge tops credit up when it runs low.
type AutoRecharge struct {
	Enabled bool `json:"enabled"`
	// ThresholdCents: recharge when available credit drops below this.
	ThresholdCents int64 `json:"threshold_cents"`
	// AmountCents is how much each recharge adds.
	AmountCents int64 `json:"amount_cents"`
	// MonthlyCapCents caps recharges per calendar month; nil for none.
	MonthlyCapCents *int64  `json:"monthly_cap_cents"`
	Pending         bool    `json:"pending"`
	LastError       *string `json:"last_error"`
}

// CreditBuckets are where the balance comes from.
type CreditBuckets struct {
	// PlanUSD is this period's subscription credit; it does not roll over.
	PlanUSD       float64    `json:"plan_usd"`
	PlanExpiresAt *time.Time `json:"plan_expires_at"`
	// PurchasedUSD never expires.
	PurchasedUSD   float64    `json:"purchased_usd"`
	PromoUSD       float64    `json:"promo_usd"`
	PromoExpiresAt *time.Time `json:"promo_expires_at"`
}

// CreditMonth is this calendar month's spending.
type CreditMonth struct {
	// Period is YYYY-MM.
	Period           string  `json:"period"`
	SpentUSD         float64 `json:"spent_usd"`
	AutoRechargedUSD float64 `json:"auto_recharged_usd"`
}

// Subscription is the plan subscription's state.
type Subscription struct {
	Status            *string    `json:"status"`
	CurrentPeriodEnd  *time.Time `json:"current_period_end"`
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
}

// CreditAccount is the organization's credit.
type CreditAccount struct {
	// Mode is prepaid or invoiced.
	Mode string `json:"mode"`
	// AvailableUSD is what new jobs can spend: the balance minus credit
	// reserved for running jobs.
	AvailableUSD float64       `json:"available_usd"`
	BalanceUSD   float64       `json:"balance_usd"`
	ReservedUSD  float64       `json:"reserved_usd"`
	Credit       CreditBuckets `json:"credit"`
	ThisMonth    CreditMonth   `json:"this_month"`
	// MonthlyLimitCents stops spending at this much per month; nil for none.
	MonthlyLimitCents *int64        `json:"monthly_limit_cents"`
	AutoRecharge      AutoRecharge  `json:"auto_recharge"`
	Subscription      *Subscription `json:"subscription"`
	// PaymentMethod labels the saved card, e.g. "Visa •••• 4242".
	PaymentMethod *string `json:"payment_method"`
}

// Billing is the plan, credit, spending controls and this month's usage.
type Billing struct {
	Plan    Plan          `json:"plan"`
	Rates   RateCard      `json:"rates"`
	Account CreditAccount `json:"account"`
	// Period is YYYY-MM.
	Period       string    `json:"period"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	UsageMinutes float64   `json:"usage_minutes"`
	UsageUSD     float64   `json:"usage_usd"`
	Currency     string    `json:"currency"`
	// PaymentsEnabled is false when credit is granted by the operator.
	PaymentsEnabled bool `json:"payments_enabled"`
}

// CheckoutParams subscribe to a plan (Plan) or buy credit (CreditCents, $10
// to $10,000). Set one.
type CheckoutParams struct {
	Plan        string `json:"plan,omitempty"`
	CreditCents int64  `json:"credit_cents,omitempty"`
}

// Checkout is where to send the customer to pay.
type Checkout struct {
	// URL is nil when nothing needs paying.
	URL *string `json:"url"`
	// Changed is true when an existing subscription moved plan in place.
	Changed bool   `json:"changed"`
	Plan    string `json:"plan,omitempty"`
}

// Portal is a link to the payment portal.
type Portal struct {
	URL string `json:"url"`
}

// AutoRechargeParams change auto-recharge.
type AutoRechargeParams struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	ThresholdCents *int64 `json:"threshold_cents,omitempty"`
	// AmountCents is $10 to $10,000 (1000 to 1000000).
	AmountCents *int64 `json:"amount_cents,omitempty"`
	// MonthlyCapCents: Null removes the cap.
	MonthlyCapCents Nullable[int64] `json:"monthly_cap_cents,omitzero"`
}

// BillingSettingsParams change spending controls.
type BillingSettingsParams struct {
	// MonthlyLimitCents: Null clears the limit.
	MonthlyLimitCents Nullable[int64]     `json:"monthly_limit_cents,omitzero"`
	AutoRecharge      *AutoRechargeParams `json:"auto_recharge,omitempty"`
}

// CreditTransaction is an entry of the credit ledger.
type CreditTransaction struct {
	ID string `json:"id"`
	// Kind is trial, subscription, purchase, auto_recharge, usage, adjustment
	// or expiry.
	Kind string `json:"kind"`
	// Bucket is promo, plan, purchased or mixed.
	Bucket string `json:"bucket"`
	// AmountUSD is positive for credit added, negative for credit spent.
	AmountUSD   float64   `json:"amount_usd"`
	Description string    `json:"description"`
	JobID       *string   `json:"job_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreditTransactionListParams page the ledger.
type CreditTransactionListParams struct {
	// Limit is 1–200; default 50.
	Limit int
}

// StatementLine is one line of a statement.
type StatementLine struct {
	Description string     `json:"description"`
	Kind        string     `json:"kind"`
	CreditUSD   float64    `json:"credit_usd"`
	Date        *time.Time `json:"date,omitempty"`
	Quantity    *float64   `json:"quantity,omitempty"`
	Unit        string     `json:"unit,omitempty"`
}

// Statement is a monthly statement: credit added and the usage drawn from it.
type Statement struct {
	ID string `json:"id"`
	// Period is YYYY-MM.
	Period      string    `json:"period"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	// Status is open or closed.
	Status       string          `json:"status"`
	Lines        []StatementLine `json:"lines"`
	UsageMinutes float64         `json:"usage_minutes"`
	UsageCents   int64           `json:"usage_cents"`
	Currency     string          `json:"currency"`
}
