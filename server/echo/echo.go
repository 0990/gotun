package echo

func StartEchoServer(address string) error {
	if err := StartTCPEchoServer(address); err != nil {
		return err
	}
	return StartUDPEchoServer(address)
}
