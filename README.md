# ocenv
Tooling to easily access and switch between different OpenShift and/or OSD clusters

## Install

```bash
go get github.com/NautiluX/ocenv
```

To enable tab completion, add the following line to your `~/.bashrc` or similar init files.

```
complete -C ocenv ocenv
```

## Usage

`ocenv` can be used to log in to several OpenShift clusters at the same time. Each cluster is referred to by a user-defined alias.
`ocenv` creates a directory in `~/ocenv/` for each cluster named by the alias. It contains an `.ocenv` that will set `$KUBECONFIG` and `$OCM_CONFIG` when the environment is started.

You can run `ocenv my-cluster` to create a new environment or switch between environments. Each environment will use a separate `$KUBECONFIG` so you can easily switch between them.
`my-cluster` in this case is an alias that you can use to identify you cluster later.

Optionally, you can run `ocenv -c my-cluster-id my-cluster` to set the `$CLUSTERID` variable in the environment. This is useful to log in to OSD clusters. When using `ocm` you can use the shorthands `ocl` to log in to the cluster, `oct` to create a tunnel when inside the environment, and `ocb` to log in with the backplane plugin.

You can leave an environment by pressing `ctrl+D`.

### OCM Environment Auto-detection

You can let ocenv detect the OCM environment and select a login script based on the environment you're currently logged in.
This will spare you to pass a script with the `-l` argument each time you log in.
To use this feature, provide your login scripts in the config file `~/.ocenv.yaml` like in the following example:

```
loginScripts:
  https://api.stage.openshift.com: ocm-stage-login
  https://api.openshift.com: ocm-prod-login
  https://api.integration.openshift.com: ocm-int-login
```

### Example workflows

#### Use backplane to log in and come back later

```
$ ocenv -l prod-login.sh -c hf203489-23fsdf-23rsdf my-cluster
$ ocb # login to the cluster
$ exit # tunnel and login loop will be closed on exit
...
$ ocenv my-cluster # no need to setup and remember everything again
$ ocb # login to the cluster
$ exit
```

#### Create a temporary environment for a quick investigation

```
$ ocenv -l prod-login.sh -t -c hf203489-23fsdf-23rsdf
$ ocb # login to the cluster
$ oc get pods .... # investigate
$ exit # tunnel and login loop will be closed on exit, environment will be cleaned up.
```

#### Use KUBECONFIG outside of the env

```
$ ocenv -l prod-login.sh -t -c hf203489-23fsdf-23rsdf my-cluster
$ ocb # login to the cluster
... in some other shell ...
$ `ocenv -k my-cluster` # use KUBECONFIG from environment
$ oc get pods ...
```