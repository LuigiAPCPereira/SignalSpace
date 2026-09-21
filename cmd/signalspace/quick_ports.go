package main

import (
	"errors"
	"fmt"
	"net"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
)

// reserveQuickPorts mantém a porta administrativa ausente no modo terminal.
// A opção de painel ainda não está exposta pela CLI: sua publicação depende dos gates HTTP.
func reserveQuickPorts(panel bool) (*admin.Listeners, error) {
	return reserveQuickPortsWith(panel, func() (net.Listener, error) {
		return net.Listen("tcp4", admin.PublicAddress)
	}, admin.ReserveListeners)
}

// reserveQuickPortsWith permite testar falhas de bind sem ocupar portas fixas.
// Nunca retorna uma reserva parcial: a origem do túnel só pode ser a porta pública.
func reserveQuickPortsWith(panel bool, reservePublic func() (net.Listener, error), reserveBoth func() (*admin.Listeners, error)) (*admin.Listeners, error) {
	if panel {
		listeners, err := reserveBoth()
		if err != nil || listeners == nil || listeners.Public == nil || listeners.Admin == nil {
			if listeners != nil {
				_ = listeners.Close()
			}
			if err != nil {
				return nil, fmt.Errorf("bind Quick panel loopback: %w", err)
			}
			return nil, errors.New("incomplete Quick panel listener reservation")
		}
		return listeners, nil
	}
	public, err := reservePublic()
	if err != nil {
		return nil, fmt.Errorf("bind diagnostic loopback: %w", err)
	}
	if public == nil {
		return nil, errors.New("missing Quick public listener")
	}
	return &admin.Listeners{Public: public}, nil
}
