package api

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"vaijunto/internal/estado"
)

func TestIniciarComPeersContextEncerraServidor(t *testing.T) {
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
		resultado <- IniciarComPeersContext(ctx, endereco, "empresa-a", repo, nil)
	}()

	cliente := &http.Client{Timeout: 200 * time.Millisecond}
	limite := time.Now().Add(3 * time.Second)
	for {
		resp, reqErr := cliente.Get("http://" + endereco + "/v1/health")
		if reqErr == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(limite) {
			cancelar()
			t.Fatalf("API nao iniciou a tempo: %v", reqErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelar()
	select {
	case err := <-resultado:
		if err != nil {
			t.Fatalf("encerramento deveria ser normal: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("API nao encerrou depois do cancelamento do contexto")
	}
}
