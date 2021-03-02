all:
	@cat banner.txt
	@echo "This Makefile only used for the banner and the logo at the moment."
	@echo "To build Gorror just use the standard go workflow."

.PHONY: banner.txt
banner.txt:
	toilet -f ghoulish -S Gorror | head -n -1 > $@

draw-banner.txt: banner.txt
	echo -n 'text 0,20 "' > $@
	cat $< >> $@
	echo '"' >> $@

logo.png: draw-banner.txt
	convert -size 475x132 xc:white -transparent white -font "Hack-Bold" \
		-pointsize 17 -fill green -draw @$< $@
	rm $<
