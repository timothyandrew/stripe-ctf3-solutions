def binary_search(container, item)
  return nil if item.nil?
  low = 0
  high = container.size - 1
  while low <= high
    mid = (low + high) / 2
    val = container[mid]
    if val > item
      high = mid - 1
    elsif val < item
      low = mid + 1
    else
      return val
    end
  end
  nil
end

path = ARGV.length > 0 ? ARGV[0] : '/usr/share/dict/words'
entries = File.read(path).split("\n").sort

contents = $stdin.read
output = contents.gsub(/[^ \n]+/) do |word|
  if binary_search(entries, word.downcase).nil?
    "<#{word}>"
  else
    word
  end
end
print output
