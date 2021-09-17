# ocenv
Tooling to easily access and switch between different OpenShift and/or OSD clusters

## Dependencies

* [direnv](https://direnv.net/)

## Install

```bash
go get github.com/NautiluX/ocenv
```

## Usage

`ocenv` can be used to log in to several OpenShift clusters at the same time. Each cluster is referred to by a user-defined alias.
`ocenv` creates a directory in `~/ocenv/` for each cluster named by the alias. It contains an `.envrc` to set `$KUBECONFIG` and `$OCM_CONFIG` when the user enters the directory.

You can run `ocenv my-cluster` to create a new environment or switch between environments. Each environment will use a separate `$KUBECONFIG` so you can easily switch between them.

Optionally, you can run `ocenv -c my-cluster-id` to set the `$CLUSTERID` variable in the environment. This is useful to log in to OSD clusters. When using `ocm` you can use the shorthands `ocl` to log in to the cluster, and `oct` to create a tunnel when inside the environment, and `ocb` to log in with the backplane plugin.

You can leave an environment by pressing `ctrl+D`.

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

