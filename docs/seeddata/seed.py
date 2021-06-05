from chance import chance
import json
import requests
from requests.auth import HTTPBasicAuth

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
  # print("Initial login details")
  # print(user['username'])
  # print(user['password'])
  # posturl = 'http://host.docker.internal:10000/auth/signup'
  posturl = 'http://localhost:10000/auth/signup'
  response = requests.post(posturl, data=user)
  resp = response.json()
  userdata = resp['Data']
  # loginurl = 'http://host.docker.internal:10000/auth/login'
  loginurl = 'http://localhost:10000/auth/login'
  login_user = {  "identity": user['username'], "password": user['password']}
  # print("Userdetails for POST Method")
  # print(login_user)
  login_response = requests.post(loginurl, data=login_user)
  l_response = login_response.json()
  authtoken = l_response["Data"]["Token"]
  for repo in range(10):
      repo = genRepo()
      username = userdata["username"]
      auth_headers = {'Authorization' : 'Bearer '+ authtoken}
      # url = 'http://host.docker.internal:10000/'+username
      url = 'http://localhost:10000/'+username
      response = requests.post(url, headers=auth_headers, json=repo)
