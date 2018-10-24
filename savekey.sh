k get secret -o yaml  | grep PrivateKey: | awk '{print $2}' | base64 --decode > ~/.ssh/sample-keypair
