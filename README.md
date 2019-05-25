# OpenFaaS faceblur function

This is an OpenFaaS faceblur function using the Pigo face detection library.

### Usage
To run the function locally you have to make sure OpenFaaS is up and running. Read the official documentation for more help. https://docs.openfaas.com/

Clone the repository:
```bash
$ git clone https://github.com/esimov/pigo-openfaas-faceblur
```

#### Build
```bash 
$ faas-cli build -f stack.yml --gateway=http://<GATEWAY-IP>
```

#### Deploy
```bash 
$ faas-cli deploy -f stack.yml --gateway=http://<GATEWAY-IP>
```

You can access the UI on the url provided to `--gateway`. 

![openfaas](https://user-images.githubusercontent.com/883386/58369734-563f5300-7f07-11e9-9a04-72c4d986abc3.png)

### Result
After deploying the OpenFaaS function `pigo-faceblur` will show up in the function list. You have to provide an image URL then hit invoke. This will return an image with the detected face regions blured out.

<p align="center">
<img src="https://user-images.githubusercontent.com/883386/58369719-0791b900-7f07-11e9-9914-52391da1f75b.jpg" title="OpenFaaS faceblur"/>
</p>

## License

Copyright © 2019 Endre Simo

This project is under the MIT License. See the LICENSE file for the full license text.
