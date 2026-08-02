# Search
* `deviantart.com/search?q=$QUERY` => `/search?q=$QUERY&type=all`
# Daily Deviations
* `deviantart.com` => `/dd`
# Deviations
* (`$USER_GROUP.deviantart.com/art/$ID`|`deviantart.com/$USER_GROUP/art/$ID`) => `/post/$USER_GROUP/$ID`
# Groups and users
## Main user page
* (`$USER_GROUP.deviantart.com`|`deviantart.com/$USER_GROUP`) => `/group_user?type=about&q=$USER_GROUP`
## Gallery
* (`$USER_GROUP.deviantart.com/gallery`|`deviantart.com/$USER_GROUP/gallery`) => `/group_user?type=gallery&q=$USER_GROUP`
## Favourites
* (`$USER_GROUP.deviantart.com/favourites`|`deviantart.com/$USER_GROUP/favourites`) => `/group_user?type=favourites&q=$USER_GROUP`
