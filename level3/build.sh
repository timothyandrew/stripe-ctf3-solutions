#!/bin/sh

set -e

# Add or modify any build steps you need here
cd "$(dirname "$0")"

echo "BUILDING"

mkdir ~/.gems

cat << EOF > ~/.gemrc
gemhome: $HOME/gems
gem: --no-document
gempath:
- $HOME/gems
- /usr/lib/ruby/gems/1.8
EOF

cat << EOF >> ~/.bashrc

EOF



gem install sinatra thin

