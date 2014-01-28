require 'sinatra'
require 'json'

get '/' do
	prefix = "test-cases/"
	
	output = Dir.chdir("/tmp/scratch") {
		`git grep -nH #{params[:q]}`
	}


	results = []
	output.split("\n").each do |line|
		path, line_number, *others = line.split(":")
		path.sub!(prefix, "")
		results << "#{path}:#{line_number}"
	end
	
	JSON.dump({
		success: true,
		results: results.uniq
	})
end

get '/healthcheck' do
	JSON.dump({success: true})
end

get '/isIndexed' do
	JSON.dump({success: true})
end

get '/index' do
	puts "Indexing #{params[:path]}"
	JSON.dump({success: true})
end