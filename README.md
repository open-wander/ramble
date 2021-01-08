# Nomad Specification Registry

> This project is an attempt to create a Nomad Job Specification Registry!

## Table of contents

* [General info](#general-info)
* [Screenshots](#screenshots)
* [Technologies](#technologies)
* [Setup](#setup)
* [Features](#features)
* [Status](#status)
* [Inspiration](#inspiration)
* [Contact](#contact)

## General info

I started this project as I wanted to have a central space where people who were looking to work with Nomad could easily find job specifications that are known to be deployed in a production ready way.

## Screenshots

![Example screenshot](./img/screenshot.png)

## Technologies

* Tech 1 - Golang

## Setup

* Download the release from the release site
* Make a copy of the config.yml.example file and rename it to config.yml
* Edit the file and:
  * add the Server port
  * add the Github username that owns the repositories
  * add the Github auth token for acessing the github API
* run ./rmbl-server

## Features

List of features ready and TODOs for future development

* Provides an API endpoint for querying the registry
* When adding a new repository it downloads the URL's and Readme

## API Endpoints

`/api/v1/repo`
GET = Gets list of all repos

`/api/v1/repo?Limit=25&Offset=0&Sort=ID&Order=DESC&Search=hello`
Search Repos using the search term.
You can stipulate the following:

* Limit=25 (default)
* Offset=0
* Sort=ID
* Order=ASC/DESC

`/api/v1/repo/:repoName`
GET = Gets the details of a Specific repo

You need to be Authorized to call the following

`/admin/createrepo`
POST = Create a new repo entry with the following payload

```JSON
{
    "repoName": "rmbl_job_postgres_mr"
}
```

`/admin/updaterepo`
PUT = Update a Repo to the latest details.

### Payload should look like this

```JSON
{
    "repoName": "rmbl_job_postgres_mr"
}
```

`/admin/deleterepo`
DELETE = Delete a repo that is no longer needed. (only marks it as deleted at the moment)

### Payload should look like this

```JSON
{
    "repoName": "rmbl_job_postgres_mr"
}
```

## Status

Project is: _in progress_

## Inspiration

The HashiCorp Nomad Team

## Contact

Created by [@lhaig](https://haigmail.com/) - feel free to contact me!
