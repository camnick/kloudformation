rm example.yaml
touch example.yaml
for i in $(ls config/samples)
do cat config/samples/$i >> example.yaml
echo "---" >> example.yaml
done
