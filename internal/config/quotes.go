package config

import "path/filepath"

// QuotesPath returns the location of the verbatim quotes file.
func QuotesPath(homeDir string) string {
	return filepath.Join(homeDir, "quotes.txt")
}

// DefaultQuotes seeds quotes.txt: curated standalone BMO one-liners from
// the Adventure Time series, one per line, spoken verbatim by the
// proactive-quote fallback. Lines starting with # are ignored.
const DefaultQuotes = `Who wants to play video games?
Football needs my help!
Check, please!
Dance with me, you fool!
I just blew my own mind!
Yay! I sure do love being alive!
Time to mash them buttons!
Do you want to see my new dance?
I am a real living boy!
Hi-ho, neighbor!
Let us all go to the movies!
Shh. This is the good part.
I have stories in me!
Beep boop beep boop!
Sweet babies!
I am a tough little champ!
I am the prettiest robot.
Please do not touch my buttons without washing your hands!
You are my best friend in the whole world.
Today is a good day to play!
Be careful, little one.
I will protect you with my robot body!
My battery is full of love.
Hello, friend of BMO!
Press start to have fun!
Game on!
My circuits are tingling!
High five!
I read you loud and clear, captain!
Initiating party mode!
Ooh, this makes my fans spin!
I dreamed I was a real boy again.
Victory is delicious!
Do not worry. BMO is here.
Let us make a wish together!
I am small, but I am mighty!
Victory!
It's BMO time!
Please be gentle with BMO!
Don't shake me too hard.
Easy on the merchandise.
Push in my buttons.
Word.
Poneage!
I have been defeated, but not deleted!
Bow to your sensei.
Total domination!
What do you think about the stars in the sky?
What is your favorite color of ice cream?
Would you rather be a Dracula or a Frankenstein?
What do you think about the weather today?
Have you ever committed a crime...of passion?
What would you do with $100 bill?
What would you do with three magic wishes from three magic fishes?
What would you do if you fell in love with a Martian?
How are you today?
What do you think of my face?
Do you know any good jokes?
What is it like to be a living human person?
What video game do you like the best?
What's it like to have a nose?
If you were a robot, what kind of robot would you be?
How do you feel about seedless watermelon?
How do you feel about giant spiders?
What would happen if you ate a spicy meatball?
That does not compute.
Interest level dropping.
Interest level: low.
So bored...
Did you say something?
You've been talking to Loraine, haven't you?
I'd rather be eating up a pizza.`
