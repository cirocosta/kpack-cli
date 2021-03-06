## kp clusterstore create

Create a cluster store

### Synopsis

Create a cluster-scoped buildpack store by providing command line arguments.

Buildpackages will be uploaded to the canonical repository.
Therefore, you must have credentials to access the registry on your machine.

This clusterstore will be created only if it does not exist.
The canonical repository is read from the "canonical.repository" key in the "kp-config" ConfigMap within "kpack" namespace.


```
kp clusterstore create <store> -b <buildpackage> [-b <buildpackage>...] [flags]
```

### Examples

```
kp clusterstore create my-store -b my-registry.com/my-buildpackage
kp clusterstore create my-store -b my-registry.com/my-buildpackage -b my-registry.com/my-other-buildpackage
kp clusterstore create my-store -b ../path/to/my-local-buildpackage.cnb
```

### Options

```
  -b, --buildpackage stringArray       location of the buildpackage
      --dry-run                        only print the object that would be sent, without sending it
  -h, --help                           help for create
      --output string                  output format. supported formats are: yaml, json
      --registry-ca-cert-path string   add CA certificates for registry API (format: /tmp/ca.crt)
      --registry-verify-certs          set whether to verify server's certificate chain and host name (default true)
```

### SEE ALSO

* [kp clusterstore](kp_clusterstore.md)	 - ClusterStore Commands

