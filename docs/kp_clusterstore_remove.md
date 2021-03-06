## kp clusterstore remove

Remove buildpackage(s) from cluster store

### Synopsis

Removes existing buildpackage(s) from a specific cluster-scoped buildpack store.

This relies on the image(s) specified to exist in the store and removes the associated buildpackage(s)


```
kp clusterstore remove <store> -b <buildpackage> [-b <buildpackage>...] [flags]
```

### Examples

```
kp clusterstore remove my-store -b my-registry.com/my-buildpackage/buildpacks_httpd@sha256:7a09cfeae4763207b9efeacecf914a57e4f5d6c4459226f6133ecaccb5c46271
kp clusterstore remove my-store -b my-registry.com/my-buildpackage/buildpacks_httpd@sha256:7a09cfeae4763207b9efeacecf914a57e4f5d6c4459226f6133ecaccb5c46271 -b my-registry.com/my-buildpackage/buildpacks_nginx@sha256:eacecf914a57e4f5d6c4459226f6133ecaccb5c462717a09cfeae4763207b9ef

```

### Options

```
  -b, --buildpackage stringArray   buildpackage to remove
  -h, --help                       help for remove
```

### SEE ALSO

* [kp clusterstore](kp_clusterstore.md)	 - ClusterStore Commands

