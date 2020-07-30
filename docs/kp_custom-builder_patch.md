## kp custom-builder patch

Patch an existing custom builder configuration

### Synopsis

 

```
kp custom-builder patch <name> [flags]
```

### Examples

```
kp cb patch my-builder
```

### Options

```
  -h, --help               help for patch
  -n, --namespace string   kubernetes namespace
  -o, --order string       path to buildpack order yaml
  -s, --stack string       stack resource to use
      --store string       buildpack store to use
```

### SEE ALSO

* [kp custom-builder](kp_custom-builder.md)	 - Custom Builder Commands

###### Auto generated by spf13/cobra on 30-Jul-2020