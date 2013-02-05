use strict;
use warnings;

my $header_size = 24;
my $header;
my $buf;
my $data;
my $i = 0;
my $lines = 1;
while ( read(*STDIN, $header, $header_size) ) {
	$i ++;
	my ($version, $code, $length, $ukn, $sequence) = unpack("nnNNN", $header);
	if ( $length > 0 ) {
		read(*STDIN, $data, $length);
		next if not $code == 128;
#		my @f = unpack("(b18)*", $data);
#		my @f = unpack("(h4)*", $data);
		my (@t) = unpack("(VV)*", $data);
#		print @t, "\n";
#		printf("%5d %2d %5d (%2d): ", $code, $length, $sequence, scalar @f);
		for (my $i = 0; $i < scalar @t; $i+=2) {#			print $x;
		#	printf("%20s", $x)
#			printf("%10s ", unpack("h*",$x));
#			printf("%10d", unpack("V", $x))
			my $flags = ($t[$i+1] >> 18) & (1 << 14) - 1;
			my $value = ($t[$i+1] >> 0) & (1 << 18) - 1;
#			printf("%.2f %d %d\n", $t[$i]/1000000, $value, $flags) if $flags == 3;
			printf("%.2f %7d %7d %s\n", $t[$i]/1000000, $value, $flags, unpack("b32", pack("V", $t[$i+1])));
#		print "\n";
#			print "\n" if $lines % 5 == 0;
			$lines ++;
		}
#		printf("%s", unpack("h".$length*2, $data));
	}
}


