# Enable auto indexing

First, [deploy executors](https://docs.sourcegraph.com/admin/deploy_executors) targeting your instance. Then, add the following to your site-config:

```yaml
{
  "codeIntelAutoIndexing.enabled": true
}
```

<!--
Olaf:

I'm going to focus on https://github.com/sourcegraph/sourcegraph/issues/24743, which is what also needs to be 
documented here. The UI already works (but the auto-indexer doesn't yet respect the configuration in the database).

The ./configure_data_retention.md doc is basically what we need to document, but instead explain about how we 
configure indexing rules rather than data retention rules for a branch or tag of a repo.

Please feel free to rework any part of the docs flow around this, including deployment and terraform-*-executors 
repo documentation, or unrelated codeintel features you want to make changes to in a drive-by fashion.
-->
