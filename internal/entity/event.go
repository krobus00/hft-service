package entity

type Publisher interface {
	JetstreamEventInit() error
}

type Subscriber interface {
	JetstreamEventSubscribe() error
}
