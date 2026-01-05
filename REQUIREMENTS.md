# Resource Requirements
To successfully list your Nomad Jobs and Packs in the RMBL Registry, ensure your repositories adhere to the following minimum file structure requirements.

## Nomad Packs
A Nomad Pack is a templated job specification. For the registry to correctly index and display your pack, the following files are required in the root of your pack directory (or the path specified when adding the resource):

## Minimum Required Files
`metadata.hcl`: This file contains the pack's metadata, such as name, description, and version.

```hcl
pack {
  name        = "my_pack"
  description = "A description of my pack"
  version     = "1.0.0"
}
```

`README.md`: A markdown file providing documentation for your pack. This content will be displayed on the registry UI.

`templates`/: A directory containing your Nomad job templates (e.g., my_job.nomad.tpl).

## Optional but Recommended

`variables.hcl`: Defines the input variables for your pack.

`outputs.tpl`: A template file that defines the output messages displayed to the user after a successful deployment.

`deps/`: A directory for pack dependencies (managed by nomad-pack).

## Nomad Jobs

A Nomad Job is a specific instance of a job specification.

## Minimum Required Files

**Job File**: A valid Nomad job specification file ending in .nomad or .nomad.hcl (e.g., `redis.nomad.hcl`).

`README.md` (Recommended): Providing a README file in the same directory (or root) ensures your job has proper documentation on the registry.

## Directory Structure Example (Pack)

```bash
my_pack/
  ├── metadata.hcl      (Required)
  ├── README.md         (Required)
  ├── variables.hcl     (Optional)
  ├── outputs.tpl       (Optional)
  └── templates/        (Required)
      └── my_job.nomad.tpl
```

## Versioning & Releases

The RMBL Registry uses Git Tags to manage versions for both Jobs and Packs.

**Git Tags**: To create a new version of your resource in the registry, push a git tag to your repository (e.g., `v1.0.0` or `0.5.2`).

**Webhooks**: If you have configured webhooks, the registry will automatically detect the new tag and create a corresponding version entry, fetching the README and content associated with that specific tag.

**Manual Updates**: You can also manually add versions via the UI if you prefer not to use webhooks.
For Nomad Packs, it is recommended that your Git tag matches the version defined in your metadata.hcl file, though the registry primarily relies on the Git tag for indexing.

## Using RMBL with Nomad Pack
You can use RMBL as a registry for the nomad-pack CLI tool. This allows you to easily discover and run packs directly from the registry.

### Adding the Global Registry

To add the entire RMBL catalog as a registry in your local environment:

```bash
nomad-pack registry add rmbl http://rmbl.openwander.org
```

## Adding a User Registry

If you only want to subscribe to a specific user's or organization's packs:

```bash
# Example for user 'lhaig'
nomad-pack registry add lhaig http://rmbl.openwander.org/lhaig
```

## Running a Pack

Once the registry is added, you can run packs by name:

```bash
nomad-pack run <pack-name> --registry=rmbl
```

You can view all available registries on the Registries page.