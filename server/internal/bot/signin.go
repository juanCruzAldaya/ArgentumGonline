package bot

import (
	"fmt"

	"juegito/server/internal/protocol"
	"juegito/server/internal/transport"
)

// signIn registers this bot's account and signs in, tolerating the name already
// existing from an earlier run.
//
// The email is required by the server and is deliberately obvious junk on a
// .invalid domain, which RFC 2606 reserves precisely so that nothing anybody
// writes can ever reach a real mailbox.
func SignIn(conn transport.Conn, codec protocol.JSONCodec, name, password string) error {
	send := func(register bool) error {
		login := protocol.Login{Name: name, Password: password, Register: register}
		if register {
			login.Email = name + "@bots.invalid"
		}
		frame, err := codec.Encode(protocol.TypeLogin, login)
		if err != nil {
			return err
		}
		return conn.Send(frame)
	}

	if err := send(true); err != nil {
		return fmt.Errorf("registro: %w", err)
	}

	// The server answers a login with an account card on success and an error
	// on failure, and keeps the connection open either way so a typo does not
	// cost a reconnect. Two attempts is all this needs: register, then sign in.
	for attempts := 0; attempts < 2; attempts++ {
		frame, err := conn.Recv()
		if err != nil {
			return fmt.Errorf("login: %w", err)
		}
		typ, _, err := codec.DecodeEnvelope(frame)
		if err != nil {
			continue
		}
		switch typ {
		case protocol.TypeAccount:
			return nil
		case protocol.TypeError:
			// Almost certainly "ese nombre ya está tomado" from a previous run.
			if err := send(false); err != nil {
				return fmt.Errorf("login: %w", err)
			}
		}
	}
	return fmt.Errorf("login rechazado para %s", name)
}
