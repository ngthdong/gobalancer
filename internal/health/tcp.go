package health

import (
    "context"
    "net"
)

type TCPChecker struct{}

func (t *TCPChecker) Check(ctx context.Context, addr string) error {
    var d net.Dialer
    conn, err := d.DialContext(ctx, "tcp", addr)
    if err != nil {
        return err
    }
    conn.Close()
    return nil
}