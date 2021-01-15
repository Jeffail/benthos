package integration

import (
	"github.com/ory/dockertest/v3"
	"log"
	"strconv"
	"testing"
)

var _ = registerIntegrationTest("sftp", func(t *testing.T) {
	//t.Skip()

	t.Parallel()

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "atmoz/sftp",
		Tag:        "alpine",
		Cmd: []string{
			"foo:pass:1001:100:upload",
		},
	})

	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}

	var sshPort int

	if err := pool.Retry(func() error {
		sshPort, err = strconv.Atoi(resource.GetPort("22/tcp"))
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	resource.Expire(900)

	t.Run("sftp", func(t *testing.T) {
		template := `
output:
  sftp:
    server: localhost
    port: $VAR1
    filepath: upload/file-$ID.txt
    credentials:
        username: foo
        secret: pass
    max_in_flight: 1

input:
  sftp:
    server: localhost
    port: $VAR1
    filepath: upload/file-$ID.txt
    credentials:
        username: foo
        secret: pass
    max_connection_attempts: 10
    directory_mode: false
    codec: lines
    delete_objects: false
`
		integrationTests(
			//integrationTestOpenCloseIsolated(),
			integrationTestStreamIsolated(2),
		).Run(
			t, template,
			testOptVarOne(strconv.Itoa(sshPort)),
		)
	})
})
