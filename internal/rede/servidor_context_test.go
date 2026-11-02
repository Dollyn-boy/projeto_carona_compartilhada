package rede

import (
	"context"
	"net"
	"testing"
	"time"

	"vaijunto/internal/estado"
)

func TestIniciarComBuscaContextEncerraConexoesAtivas(t *testing.T) {
	reserva, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endereco := reserva.Addr().String()
	_ = reserva.Close()

	repo, err := estado.NovoRepositorioDeEmpresa("empresa-a")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	resultado := make(chan error, 1)
	go func() {
		resultado <- IniciarComBuscaContext(ctx, endereco, repo, nil)
	}()

	var conn net.Conn
	limite := time.Now().Add(3 * time.Second)
	for {
		conn, err = net.DialTimeout("tcp", endereco, 100*time.Millisecond)
		if err == nil {
			break
		}
		if time.Now().After(limite) {
			cancelar()
			t.Fatalf("servidor TCP nao iniciou a tempo: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer conn.Close()

	cancelar()
	select {
	case err := <-resultado:
		if err != nil {
			t.Fatalf("encerramento deveria ser normal: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("servidor TCP nao encerrou apos o cancelamento")
	}
}
