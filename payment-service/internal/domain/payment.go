package domain

type PaymentStatus string

const (
	PaymentStatusAuthorized PaymentStatus = "Authorized"
	PaymentStatusDeclined   PaymentStatus = "Declined"
)

type Payment struct {
	ID            string        `json:"id"`
	OrderID       string        `json:"order_id"`
	TransactionID string        `json:"transaction_id"`
	Amount        int64         `json:"amount"`
	Status        PaymentStatus `json:"status"`
	CustomerEmail string        `json:"customer_email"`
}
