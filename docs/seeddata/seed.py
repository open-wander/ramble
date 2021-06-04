from chance import chance
import json
import requests

def genUser():
  username = chance.first()
  email = chance.email()
  password = chance.string(minimum=5, maximum=20)
  user = {"username": username, "email": email, "password": password}
  return user

def genRepo():
  name = chance.word(language='en')
  description = chance.sentence()
  version = chance.character(pool='vV') + chance.character(pool='012345') + "." + chance.character(pool='012345') + "." + chance.character(pool='012345')
  url = chance.url(dom='github.com', exts=['hcl', 'nomad'])
  repo = {"name": name, "description": description, "version": version, "url": url}
  return repo

for u in range(40):
  user = genUser()
  print(user['username'])
  print(user['password'])
  posturl = 'http://host.docker.internal:10000/auth/signup'
  # posturl = 'http://localhost:10000/auth/signup'
  response = requests.post(posturl, data=user)
  resp = response.json()
  userdata = resp['Data']
  for repo in range(10):
      repo = genRepo()
      username = userdata["username"]
      url = 'http://host.docker.internal:10000/'+username
      # url = 'http://localhost:10000/'+username
      response = requests.post(url, json=repo)
