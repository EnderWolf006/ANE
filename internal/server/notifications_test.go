package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

// Exercise the actual SMTP transport against a local relay; no mail leaves the test process.
func TestSMTPTransport(t *testing.T) {
	for _, security := range []string{"none", "starttls", "plaintext-auth"} {
		t.Run(security, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			messages := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				reader := textproto.NewReader(bufio.NewReader(conn))
				fmt.Fprint(conn, "220 local ANE test relay\r\n")
				for {
					line, err := reader.ReadLine()
					if err == io.EOF {
						done <- nil
						return
					}
					if err != nil {
						done <- err
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						fmt.Fprint(conn, "250-localhost\r\n250 PIPELINING\r\n")
					case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
						fmt.Fprint(conn, "250 ok\r\n")
					case line == "DATA":
						fmt.Fprint(conn, "354 send content\r\n")
						data, err := reader.ReadDotBytes()
						if err != nil {
							done <- err
							return
						}
						messages <- string(data)
						fmt.Fprint(conn, "250 accepted\r\n")
					case line == "QUIT":
						fmt.Fprint(conn, "221 bye\r\n")
						done <- nil
						return
					default:
						done <- fmt.Errorf("unexpected SMTP command: %s", line)
						return
					}
				}
			}()
			config := SMTPConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Security: security, FromAddress: "ane@example.test", FromName: "ANE"}
			if security == "plaintext-auth" {
				config.Security = "none"
				config.Username = "must-not-leak"
				config.Password = "secret"
			}
			body := strings.Repeat("收款通知：测试通过扫码向你付款9.99元\n", 20)
			err = sendSMTP(context.Background(), config, "merchant@example.test", "ANE · 收款成功", body)
			if security == "none" {
				if err != nil {
					t.Fatal(err)
				}
				var content string
				select {
				case content = <-messages:
				case <-time.After(3 * time.Second):
					t.Fatal("relay received no message")
				}
				message, err := mail.ReadMessage(strings.NewReader(content))
				if err != nil {
					t.Fatal(err)
				}
				subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
				if err != nil || subject != "ANE · 收款成功" {
					t.Fatal("invalid UTF-8 subject", subject, err)
				}
				decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
				if err != nil || string(decoded) != body {
					t.Fatal("body corrupted", err)
				}
				for _, line := range strings.Split(content, "\n") {
					if len(line) > 998 {
						t.Fatal("SMTP line exceeds limit")
					}
				}
			} else {
				if err == nil {
					t.Fatal("insecure SMTP configuration accepted")
				}
				if security == "starttls" && !strings.Contains(err.Error(), "STARTTLS") {
					t.Fatal(err)
				}
				if security == "plaintext-auth" && !strings.Contains(err.Error(), "requires TLS") {
					t.Fatal(err)
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
