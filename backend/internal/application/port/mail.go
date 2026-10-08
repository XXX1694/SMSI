package port

import "context"

// Message is one transactional email. Text and HTML are the rendered bodies
// (multipart/alternative); Template names the template they came from and is
// the only body-related field that is safe to log.
type Message struct {
	To       string
	Subject  string
	Text     string
	HTML     string
	Template string
}

// Mailer delivers a message synchronously. It runs in the worker.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// MailQueue hands a message to the worker for delivery. Callers on the request
// path use it so that a slow or failing mail provider never affects a response.
type MailQueue interface {
	Enqueue(ctx context.Context, m Message) error
}
