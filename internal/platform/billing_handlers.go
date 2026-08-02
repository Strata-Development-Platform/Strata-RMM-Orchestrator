package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (s *APIServer) handleGetBillingAccount(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var id, provider, providerCustomerID, providerSubscriptionID string
	var paymentProviderID, billingCycle, status, billingEmail string
	var createdAt, updatedAt time.Time

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, msp_id, provider, provider_customer_id, provider_subscription_id,
		       payment_provider_id, billing_cycle, status, billing_email, created_at, updated_at
		FROM billing_accounts WHERE msp_id = $1
	`, mspID).Scan(&id, &mspID, &provider, &providerCustomerID, &providerSubscriptionID,
		&paymentProviderID, &billingCycle, &status, &billingEmail, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "billing account not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	account := map[string]interface{}{
		"id":                       id,
		"msp_id":                   mspID,
		"provider":                 provider,
		"provider_customer_id":     providerCustomerID,
		"provider_subscription_id": providerSubscriptionID,
		"payment_provider_id":      paymentProviderID,
		"billing_cycle":            billingCycle,
		"status":                   status,
		"billing_email":            billingEmail,
		"created_at":               createdAt,
		"updated_at":               updatedAt,
	}

	writeJSON(w, http.StatusOK, account)
}

func (s *APIServer) handleCreateBillingAccount(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var req struct {
		Provider           string `json:"provider"`
		ProviderCustomerID string `json:"provider_customer_id"`
		BillingCycle       string `json:"billing_cycle"`
		BillingEmail       string `json:"billing_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Provider == "" || req.ProviderCustomerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider and provider_customer_id required"})
		return
	}
	if req.BillingCycle == "" {
		req.BillingCycle = "monthly"
	}
	if req.BillingCycle != "monthly" && req.BillingCycle != "annual" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "billing_cycle must be monthly or annual"})
		return
	}

	var id string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO billing_accounts (msp_id, provider, provider_customer_id, billing_cycle, billing_email)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (msp_id) DO UPDATE SET
			provider = $2,
			provider_customer_id = $3,
			billing_cycle = $4,
			billing_email = $5,
			updated_at = NOW()
		RETURNING id
	`, mspID, req.Provider, req.ProviderCustomerID, req.BillingCycle, req.BillingEmail).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "billing account created/updated",
	})
}

func (s *APIServer) handleDeleteBillingAccount(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	_, err := s.requestDB(r).ExecContext(r.Context(), `DELETE FROM billing_accounts WHERE msp_id = $1`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"msg": "billing account deleted"})
}

func (s *APIServer) handleGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT s.id, s.msp_id, s.plan_id, p.name as plan_name, s.status, s.billing_period,
		       s.started_at, s.current_period_end, s.cancelled_at, s.cancel_at_period_end,
		       s.created_at, s.updated_at
		FROM subscriptions s
		JOIN plans p ON s.plan_id = p.id
		WHERE s.msp_id = $1
		ORDER BY s.started_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var subscriptions []map[string]interface{}
	for rows.Next() {
		var id, mspID, planID, planName, status, billingPeriod string
		var startedAt, currentPeriodEnd, cancelledAt time.Time
		var cancelAtPeriodEnd bool
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &mspID, &planID, &planName, &status,
			&billingPeriod, &startedAt, &currentPeriodEnd,
			&cancelledAt, &cancelAtPeriodEnd, &createdAt, &updatedAt)
		if err != nil {
			continue
		}

		sub := map[string]interface{}{
			"id":                   id,
			"msp_id":               mspID,
			"plan_id":              planID,
			"plan_name":            planName,
			"status":               status,
			"billing_period":       billingPeriod,
			"started_at":           startedAt,
			"current_period_end":   currentPeriodEnd,
			"cancelled_at":         cancelledAt,
			"cancel_at_period_end": cancelAtPeriodEnd,
			"created_at":           createdAt,
			"updated_at":           updatedAt,
		}
		subscriptions = append(subscriptions, sub)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"subscriptions": subscriptions})
}

func (s *APIServer) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var req struct {
		PlanID        string `json:"plan_id"`
		BillingPeriod string `json:"billing_period"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.PlanID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id required"})
		return
	}
	if req.BillingPeriod == "" {
		req.BillingPeriod = "monthly"
	}
	if req.BillingPeriod != "monthly" && req.BillingPeriod != "annual" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "billing_period must be monthly or annual"})
		return
	}

	var planID string
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT id FROM plans WHERE id = $1`, req.PlanID).Scan(&planID)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var id string
	periodEnd := time.Now().AddDate(0, 1, 0)
	if req.BillingPeriod == "annual" {
		periodEnd = time.Now().AddDate(1, 0, 0)
	}
	err = s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO subscriptions (msp_id, plan_id, billing_period, current_period_end)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, mspID, planID, req.BillingPeriod, periodEnd).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "subscription created",
	})
}

func (s *APIServer) handleCancelSubscription(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("subscriptionID")
	mspID := r.PathValue("mspID")

	var currentPeriodEnd time.Time
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT current_period_end FROM subscriptions WHERE id = $1 AND msp_id = $2`, subscriptionID, mspID).Scan(&currentPeriodEnd)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE subscriptions SET
			cancel_at_period_end = true,
			cancelled_at = NOW(),
			status = 'cancelled'
		WHERE id = $1 AND msp_id = $2
	`, subscriptionID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   subscriptionID,
		"cancel_at_period_end": true,
		"cancelled_at":         time.Now(),
		"msg":                  "subscription cancelled at period end",
	})
}

func (s *APIServer) handleGetInvoices(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, msp_id, subscription_id, invoice_number, period_start, period_end,
		       subtotal, tax, total, currency, status, paid_at, due_at, invoice_pdf_url,
		       external_invoice_id, created_at
		FROM invoices WHERE msp_id = $1
		ORDER BY created_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var invoices []map[string]interface{}
	for rows.Next() {
		var id, mspID, subscriptionID, invoiceNumber string
		var periodStart, periodEnd, paidAt, dueAt, createdAt time.Time
		var subtotal, tax, total float64
		var currency, status, invoicePdfURL, externalInvoiceID string

		err := rows.Scan(&id, &mspID, &subscriptionID, &invoiceNumber,
			&periodStart, &periodEnd, &subtotal, &tax, &total,
			&currency, &status, &paidAt, &dueAt, &invoicePdfURL,
			&externalInvoiceID, &createdAt)
		if err != nil {
			continue
		}

		inv := map[string]interface{}{
			"id":                  id,
			"msp_id":              mspID,
			"subscription_id":     subscriptionID,
			"invoice_number":      invoiceNumber,
			"period_start":        periodStart,
			"period_end":          periodEnd,
			"subtotal":            subtotal,
			"tax":                 tax,
			"total":               total,
			"currency":            currency,
			"status":              status,
			"paid_at":             paidAt,
			"due_at":              dueAt,
			"invoice_pdf_url":     invoicePdfURL,
			"external_invoice_id": externalInvoiceID,
			"created_at":          createdAt,
		}
		invoices = append(invoices, inv)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"invoices": invoices})
}

func (s *APIServer) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID := r.PathValue("invoiceID")
	mspID := r.PathValue("mspID")

	var id, mspID2, subscriptionID, invoiceNumber string
	var periodStart, periodEnd, paidAt, dueAt, createdAt time.Time
	var subtotal, tax, total float64
	var currency, status, invoicePdfURL, externalInvoiceID string

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, msp_id, subscription_id, invoice_number, period_start, period_end,
		       subtotal, tax, total, currency, status, paid_at, due_at, invoice_pdf_url,
		       external_invoice_id, created_at
		FROM invoices WHERE id = $1 AND msp_id = $2
	`, invoiceID, mspID).Scan(&id, &mspID2, &subscriptionID, &invoiceNumber,
		&periodStart, &periodEnd, &subtotal, &tax, &total,
		&currency, &status, &paidAt, &dueAt, &invoicePdfURL,
		&externalInvoiceID, &createdAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invoice not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	inv := map[string]interface{}{
		"id":                  id,
		"msp_id":              mspID,
		"subscription_id":     subscriptionID,
		"invoice_number":      invoiceNumber,
		"period_start":        periodStart,
		"period_end":          periodEnd,
		"subtotal":            subtotal,
		"tax":                 tax,
		"total":               total,
		"currency":            currency,
		"status":              status,
		"paid_at":             paidAt,
		"due_at":              dueAt,
		"invoice_pdf_url":     invoicePdfURL,
		"external_invoice_id": externalInvoiceID,
		"created_at":          createdAt,
	}

	writeJSON(w, http.StatusOK, inv)
}

func (s *APIServer) handleSubmitUsage(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var req struct {
		MeterName  string `json:"meter_name"`
		Quantity   int    `json:"quantity"`
		Unit       string `json:"unit"`
		Source     string `json:"source"`
		ExternalID string `json:"external_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.MeterName == "" || req.Source == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "meter_name and source required"})
		return
	}
	if req.Quantity == 0 {
		req.Quantity = 1
	}
	if req.Unit == "" {
		req.Unit = "units"
	}

	var id string
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO usage_records (msp_id, meter_name, quantity, unit, source, external_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, mspID, req.MeterName, req.Quantity, req.Unit, req.Source, req.ExternalID).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "usage record created",
	})
}

func (s *APIServer) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")
	meterName := r.PathValue("meterName")

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, meter_name, quantity, unit, recorded_at, source, external_id
		FROM usage_records
		WHERE msp_id = $1 AND meter_name = $2
		ORDER BY recorded_at DESC
		LIMIT 100
	`, mspID, meterName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var records []map[string]interface{}
	for rows.Next() {
		var id, meterName2, unit, source, externalID string
		var quantity int
		var recordedAt time.Time

		err := rows.Scan(&id, &meterName2, &quantity, &unit, &recordedAt, &source, &externalID)
		if err != nil {
			continue
		}

		rec := map[string]interface{}{
			"id":          id,
			"meter_name":  meterName,
			"quantity":    quantity,
			"unit":        unit,
			"recorded_at": recordedAt,
			"source":      source,
			"external_id": externalID,
		}
		records = append(records, rec)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msp_id":     mspID,
		"meter_name": meterName,
		"records":    records,
		"count":      len(records),
	})
}

func (s *APIServer) handleGetPaymentMethods(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, msp_id, provider_payment_method_id, type, card_brand, last_four,
		       exp_month, exp_year, is_default, created_at
		FROM payment_methods WHERE msp_id = $1
		ORDER BY is_default DESC, created_at DESC
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var methods []map[string]interface{}
	for rows.Next() {
		var id, mspID2, providerPaymentMethodID, type2, cardBrand, lastFour string
		var expMonth, expYear int
		var isDefault bool
		var createdAt time.Time

		err := rows.Scan(&id, &mspID2, &providerPaymentMethodID, &type2, &cardBrand, &lastFour,
			&expMonth, &expYear, &isDefault, &createdAt)
		if err != nil {
			continue
		}

		m := map[string]interface{}{
			"id":                         id,
			"msp_id":                     mspID,
			"provider_payment_method_id": providerPaymentMethodID,
			"type":                       type2,
			"card_brand":                 cardBrand,
			"last_four":                  lastFour,
			"exp_month":                  expMonth,
			"exp_year":                   expYear,
			"is_default":                 isDefault,
			"created_at":                 createdAt,
		}
		methods = append(methods, m)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"payment_methods": methods})
}

func (s *APIServer) handleAddPaymentMethod(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var req struct {
		ProviderPaymentMethodID string                 `json:"provider_payment_method_id"`
		Type                    string                 `json:"type"`
		CardBrand               string                 `json:"card_brand"`
		LastFour                string                 `json:"last_four"`
		ExpMonth                int                    `json:"exp_month"`
		ExpYear                 int                    `json:"exp_year"`
		ProviderData            map[string]interface{} `json:"provider_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.ProviderPaymentMethodID == "" || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_payment_method_id and type required"})
		return
	}
	if req.Type != "card" && req.Type != "bank" && req.Type != "paypal" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be card, bank, or paypal"})
		return
	}

	var id string
	providerDataJSON, _ := json.Marshal(req.ProviderData)
	err := s.requestDB(r).QueryRowContext(r.Context(), `
		INSERT INTO payment_methods (msp_id, provider_payment_method_id, type, card_brand, last_four, exp_month, exp_year, provider_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, mspID, req.ProviderPaymentMethodID, req.Type, req.CardBrand, req.LastFour, req.ExpMonth, req.ExpYear, providerDataJSON).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  id,
		"msg": "payment method added",
	})
}

func (s *APIServer) handleSetDefaultPaymentMethod(w http.ResponseWriter, r *http.Request) {
	paymentMethodID := r.PathValue("paymentMethodID")
	mspID := r.PathValue("mspID")

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE payment_methods SET is_default = false WHERE msp_id = $1
	`, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `
		UPDATE payment_methods SET is_default = true WHERE id = $1 AND msp_id = $2
	`, paymentMethodID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msg": "default payment method updated",
	})
}

func (s *APIServer) handleDeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	paymentMethodID := r.PathValue("paymentMethodID")
	mspID := r.PathValue("mspID")

	var isDefault bool
	err := s.requestDB(r).QueryRowContext(r.Context(), `SELECT is_default FROM payment_methods WHERE id = $1 AND msp_id = $2`, paymentMethodID, mspID).Scan(&isDefault)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "payment method not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if isDefault {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete default payment method"})
		return
	}

	_, err = s.requestDB(r).ExecContext(r.Context(), `DELETE FROM payment_methods WHERE id = $1 AND msp_id = $2`, paymentMethodID, mspID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"msg": "payment method deleted"})
}

func (s *APIServer) handleGetRevenueReport(w http.ResponseWriter, r *http.Request) {
	mspID := r.PathValue("mspID")

	var totalRevenue float64
	var subscriptionRevenue float64
	var usageRevenue float64
	var refundAmount float64

	s.requestDB(r).QueryRowContext(r.Context(), `SELECT COALESCE(SUM(total), 0) FROM invoices WHERE msp_id = $1 AND status = 'paid'`, mspID).Scan(&totalRevenue)
	s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(i.total), 0) FROM invoices i
		JOIN subscriptions s ON i.subscription_id = s.id
		WHERE s.msp_id = $1 AND i.status = 'paid'
	`, mspID).Scan(&subscriptionRevenue)
	s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(u.quantity * 0.01), 0) FROM usage_records u
		WHERE u.msp_id = $1
	`, mspID).Scan(&usageRevenue)
	s.requestDB(r).QueryRowContext(r.Context(), `SELECT COALESCE(SUM(amount), 0) FROM refunds WHERE msp_id = $1`, mspID).Scan(&refundAmount)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"msp_id":               mspID,
		"total_revenue":        totalRevenue,
		"subscription_revenue": subscriptionRevenue,
		"usage_revenue":        usageRevenue,
		"refunds":              refundAmount,
		"net_revenue":          totalRevenue - refundAmount,
	})
}
