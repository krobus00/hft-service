package entity

import "context"

type Publisher interface {
	JetstreamEventInit(ctx context.Context) error
}

type Subscriber interface {
	JetstreamEventSubscribe(ctx context.Context) error
}
