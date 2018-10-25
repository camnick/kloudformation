k get secret -o yaml  | grep PrivateKey: | awk '{print $2}' | base64 --decode > ~/.ssh/$1.pem
echo "You saved the key as $1."
