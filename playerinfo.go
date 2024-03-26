package main

import (
	"io"

	"github.com/maxsupermanhd/go-vmc/v762/chat"
	pk "github.com/maxsupermanhd/go-vmc/v762/net/packet"
)

type PlayerInfoUpdateDisplayName struct {
	uuid        pk.UUID
	displayname *chat.Message
}

func (info *PlayerInfoUpdateDisplayName) ReadFrom(r io.Reader) (n int64, err error) {
	var hasdisplayname pk.Boolean
	return pk.Tuple{
		&info.uuid,
		&hasdisplayname,
		pk.Opt{
			Has: &hasdisplayname,
			Field: func() pk.FieldDecoder {
				info.displayname = new(chat.Message)
				return info.displayname
			},
		},
	}.ReadFrom(r)
}

type PlayerInfoUpdatePing struct {
	uuid pk.UUID
	ping pk.VarInt
}

func (info *PlayerInfoUpdatePing) ReadFrom(r io.Reader) (n int64, err error) {
	return pk.Tuple{
		&info.uuid,
		&info.ping,
	}.ReadFrom(r)
}

type PlayerInfoUpdateGamemode struct {
	uuid     pk.UUID
	gamemode pk.VarInt
}

func (info *PlayerInfoUpdateGamemode) ReadFrom(r io.Reader) (n int64, err error) {
	return pk.Tuple{
		&info.uuid,
		&info.gamemode,
	}.ReadFrom(r)
}

type PlayerInfoAdd struct {
	uuid         pk.UUID
	name         pk.String
	props        []PlayerInfoAddProps
	gamemode     pk.VarInt
	ping         pk.VarInt
	displayname  *chat.Message
	hassignature pk.Boolean
	timestamp    pk.Long
	pubkeylen    pk.VarInt
	pubkey       []byte
	signlen      pk.VarInt
	sign         []byte
}

func (info *PlayerInfoAdd) ReadFrom(r io.Reader) (n int64, err error) {
	var hasdisplayname pk.Boolean
	return pk.Tuple{
		&info.uuid,
		&info.name,
		pk.Array(&info.props),
		&info.gamemode,
		&info.ping,
		&hasdisplayname,
		pk.Opt{
			Has: &hasdisplayname,
			Field: func() pk.FieldDecoder {
				info.displayname = new(chat.Message)
				return info.displayname
			},
		},
		&info.hassignature,
		pk.Opt{
			Has: &hasdisplayname,
			Field: func() pk.FieldDecoder {
				return &info.timestamp
			},
		},
		pk.Opt{
			Has: &hasdisplayname,
			Field: func() pk.FieldDecoder {
				return &info.pubkeylen
			},
		},
		pk.Opt{
			Has: func() bool { return info.pubkeylen != 0 },
			Field: func() pk.FieldDecoder {
				return (*pk.ByteArray)(&info.pubkey)
			},
		},
		pk.Opt{
			Has: &hasdisplayname,
			Field: func() pk.FieldDecoder {
				return &info.signlen
			},
		},
		pk.Opt{
			Has: func() bool { return info.signlen != 0 },
			Field: func() pk.FieldDecoder {
				return (*pk.ByteArray)(&info.sign)
			},
		},
	}.ReadFrom(r)
}

type PlayerInfoAddProps struct {
	name      pk.String
	value     pk.String
	issigned  pk.Boolean
	signature pk.String
}

func (info *PlayerInfoAddProps) ReadFrom(r io.Reader) (n int64, err error) {
	return pk.Tuple{
		&info.name,
		&info.value,
		&info.issigned,
		pk.Opt{
			Has: &info.issigned,
			Field: func() pk.FieldDecoder {
				return &info.signature
			},
		},
	}.ReadFrom(r)
}
